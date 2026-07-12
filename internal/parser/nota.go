package parser

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/duducury/investimentos-backend/internal/dynamo"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ParseNota reads a PDF nota de negociação and returns parsed transactions.
func ParseNota(r io.Reader) ([]dynamo.Transaction, error) {
	tmp, err := os.CreateTemp("", "nota*.pdf")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		return nil, fmt.Errorf("writing temp file: %w", err)
	}
	tmp.Close()
	slog.Info("extracting text from PDF", "file", tmp.Name())

	text, err := extractText(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("extracting text: %w", err)
	}
	slog.Info("PDF text extracted", "chars", len(text))

	transactions, err := parseTransactions(text)
	if err != nil {
		return nil, err
	}
	slog.Info("transactions parsed from nota", "count", len(transactions))
	return transactions, nil
}

// ---- PDF text extraction ----

func parseHex(s string) int {
	v := 0
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		}
	}
	return v
}

var hexRe = regexp.MustCompile(`<([0-9A-Fa-f]+)>`)

func parseCMap(data []byte) map[int]rune {
	cm := map[int]rune{}
	s := string(data)
	charRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	charSec := regexp.MustCompile(`beginbfchar([\s\S]*?)endbfchar`)
	rangeRe := regexp.MustCompile(`beginbfrange([\s\S]*?)endbfrange`)
	arrayRangeRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*\[([^\]]*)\]`)
	linearRangeRe := regexp.MustCompile(`<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)

	for _, sec := range charSec.FindAllStringSubmatch(s, -1) {
		for _, m := range charRe.FindAllStringSubmatch(sec[1], -1) {
			cm[parseHex(m[1])] = rune(parseHex(m[2]))
		}
	}
	for _, sec := range rangeRe.FindAllStringSubmatch(s, -1) {
		for _, m := range arrayRangeRe.FindAllStringSubmatch(sec[1], -1) {
			start := parseHex(m[1])
			for i, hm := range hexRe.FindAllStringSubmatch(m[3], -1) {
				cm[start+i] = rune(parseHex(hm[1]))
			}
		}
		for _, line := range strings.Split(sec[1], "\n") {
			if strings.Contains(line, "[") {
				continue
			}
			m := linearRangeRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			start, end := parseHex(m[1]), parseHex(m[2])
			dst := parseHex(m[3])
			for i := start; i <= end; i++ {
				cm[i] = rune(dst + (i - start))
			}
		}
	}
	return cm
}

// buildFontCMaps returns a map from font name (e.g. "F10") to its ToUnicode charMap
// by navigating the PDF's resource dictionaries.
func buildFontCMaps(ctx *model.Context) map[string]map[int]rune {
	xref := ctx.XRefTable

	cmapByObj := map[int]map[int]rune{}
	for objNr := 0; objNr < len(xref.Table); objNr++ {
		entry := xref.Table[objNr]
		if entry == nil || entry.Object == nil || entry.Free {
			continue
		}
		sd, ok := entry.Object.(types.StreamDict)
		if !ok {
			continue
		}
		sd2 := sd
		if err := sd2.Decode(); err != nil || len(sd2.Content) == 0 {
			continue
		}
		if strings.Contains(string(sd2.Content), "beginbf") {
			cmapByObj[objNr] = parseCMap(sd2.Content)
		}
	}

	result := map[string]map[int]rune{}
	for objNr := 0; objNr < len(xref.Table); objNr++ {
		entry := xref.Table[objNr]
		if entry == nil || entry.Object == nil || entry.Free {
			continue
		}
		resDict, ok := entry.Object.(types.Dict)
		if !ok {
			continue
		}
		fontResRaw, hasFont := resDict["Font"]
		if !hasFont {
			continue
		}
		fontRes, ok := fontResRaw.(types.Dict)
		if !ok {
			continue
		}
		for fontName, refRaw := range fontRes {
			ref, ok := refRaw.(types.IndirectRef)
			if !ok {
				continue
			}
			fontEntry := xref.Table[int(ref.ObjectNumber)]
			if fontEntry == nil || fontEntry.Object == nil {
				continue
			}
			fontDict, ok := fontEntry.Object.(types.Dict)
			if !ok {
				continue
			}
			tuRaw, hasTU := fontDict["ToUnicode"]
			if !hasTU {
				continue
			}
			tuRef, ok := tuRaw.(types.IndirectRef)
			if !ok {
				continue
			}
			if cm, ok := cmapByObj[int(tuRef.ObjectNumber)]; ok {
				result[string(fontName)] = cm
			}
		}
	}
	return result
}

type glyph struct {
	x, y float64
	text string
}

var tokenRe = regexp.MustCompile(
	`<[0-9A-Fa-f]+>` +
		`|\[(?:\s*<[0-9A-Fa-f]+>\s*(?:-?[0-9]+(?:\.[0-9]+)?\s*)?)*\]` +
		`|/[^\s/\[<(]+` +
		`|-?[0-9]+(?:\.[0-9]+)?` +
		`|[A-Za-z][A-Za-z*]*`,
)

func decodeHex(hex string, cm map[int]rune) string {
	var out strings.Builder
	for i := 0; i+3 < len(hex); i += 4 {
		if r, ok := cm[parseHex(hex[i:i+4])]; ok {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func extractGlyphs(content string, fontCMaps map[string]map[int]rune) []glyph {
	var result []glyph
	tokens := tokenRe.FindAllString(content, -1)
	var stack []string
	var curX, curY float64
	inText := false
	currentFont := ""

	cm := func() map[int]rune {
		if m, ok := fontCMaps[currentFont]; ok {
			return m
		}
		merged := map[int]rune{}
		for _, m := range fontCMaps {
			for k, v := range m {
				merged[k] = v
			}
		}
		return merged
	}

	parseF := func(s string) float64 {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}

	for _, tok := range tokens {
		switch tok {
		case "BT":
			inText = true
			curX, curY = 0, 0
			stack = stack[:0]
		case "ET":
			inText = false
			stack = stack[:0]
		case "Tf":
			if len(stack) >= 2 {
				currentFont = strings.TrimPrefix(stack[len(stack)-2], "/")
			}
			stack = stack[:0]
		case "Tm":
			if len(stack) >= 6 {
				curX = parseF(stack[len(stack)-2])
				curY = parseF(stack[len(stack)-1])
			}
			stack = stack[:0]
		case "Td", "TD":
			if len(stack) >= 2 {
				curX += parseF(stack[len(stack)-2])
				curY += parseF(stack[len(stack)-1])
			}
			stack = stack[:0]
		case "T*":
			stack = stack[:0]
		case "Tj":
			if inText && len(stack) >= 1 {
				hexStr := strings.Trim(stack[len(stack)-1], "<>")
				if t := decodeHex(hexStr, cm()); t != "" {
					result = append(result, glyph{x: curX, y: curY, text: t})
				}
				stack = stack[:len(stack)-1]
			}
		case "TJ":
			if inText && len(stack) >= 1 {
				arrStr := stack[len(stack)-1]
				if strings.HasPrefix(arrStr, "[") {
					var sb strings.Builder
					for _, hm := range hexRe.FindAllStringSubmatch(arrStr, -1) {
						sb.WriteString(decodeHex(hm[1], cm()))
					}
					if t := sb.String(); t != "" {
						result = append(result, glyph{x: curX, y: curY, text: t})
					}
				}
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, tok)
		}
	}
	return result
}

func absF(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func reconstructText(glyphs []glyph) string {
	if len(glyphs) == 0 {
		return ""
	}
	type line struct {
		y      float64
		glyphs []glyph
	}
	var lines []line
	for _, g := range glyphs {
		placed := false
		for i := range lines {
			if absF(lines[i].y-g.y) < 2.0 {
				lines[i].glyphs = append(lines[i].glyphs, g)
				placed = true
				break
			}
		}
		if !placed {
			lines = append(lines, line{y: g.y, glyphs: []glyph{g}})
		}
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].y > lines[j].y
	})
	var sb strings.Builder
	for _, l := range lines {
		sort.Slice(l.glyphs, func(i, j int) bool {
			return l.glyphs[i].x < l.glyphs[j].x
		})
		prevEnd := 0.0
		for i, g := range l.glyphs {
			if i > 0 && g.x-prevEnd > 8 {
				sb.WriteString("  ")
			}
			sb.WriteString(g.text)
			prevEnd = g.x + float64(len([]rune(g.text)))*6
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func extractText(pdfPath string) (string, error) {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	ctx, err := pdfcpu.ReadFile(pdfPath, conf)
	if err != nil {
		return "", err
	}
	fontCMaps := buildFontCMaps(ctx)
	slog.Info("font CMaps loaded", "fonts", len(fontCMaps))

	tmpDir, err := os.MkdirTemp("", "nota_content")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	if err := api.ExtractContentFile(pdfPath, tmpDir, nil, conf); err != nil {
		return "", err
	}

	var sb strings.Builder
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		raw, _ := os.ReadFile(tmpDir+"/"+e.Name())
		glyphs := extractGlyphs(string(raw), fontCMaps)
		slog.Info("page processed", "page", e.Name(), "glyphs", len(glyphs))
		sb.WriteString(reconstructText(glyphs))
	}
	return sb.String(), nil
}

// ---- Transaction parsing ----

var (
	dateRe  = regexp.MustCompile(`Data de Referência:\s+(\d{2}/\d{2}/\d{4})`)
	notaRe  = regexp.MustCompile(`Nº Nota\s+\d{2}/\d{2}/\d{4}\s+(\d+)`)
	boldRe  = regexp.MustCompile(`\d+-BOVESPA\s+(C|V)\s+\S+\s+[^\n]*?([A-Z]{4}[0-9]{1,2})F?\s+@`)
	totalRe = regexp.MustCompile(`([A-Z]{4}[0-9]{1,2})F?[^\n]*?Quantidade Total:\s+(\d+)\s+Preço Médio:\s+([\d,]+)`)
)

var brt = time.FixedZone("BRT", -3*3600)

func parseTransactions(text string) ([]dynamo.Transaction, error) {
	dm := dateRe.FindStringSubmatch(text)
	if dm == nil {
		return nil, fmt.Errorf("trade date not found in nota")
	}
	date, err := time.ParseInLocation("02/01/2006", dm[1], brt)
	if err != nil {
		return nil, fmt.Errorf("invalid date %q: %w", dm[1], err)
	}

	notaNumber := ""
	if nm := notaRe.FindStringSubmatch(text); nm != nil {
		notaNumber = nm[1]
		slog.Info("nota number found", "notaNumber", notaNumber)
	} else {
		slog.Warn("nota number not found in text")
	}

	// Collect C/V per ticker from bold rows (first occurrence wins)
	cvByTicker := map[string]string{}
	for _, m := range boldRe.FindAllStringSubmatch(text, -1) {
		ticker := m[2]
		if _, exists := cvByTicker[ticker]; !exists {
			cvByTicker[ticker] = m[1]
		}
	}

	// Parse aggregated summary lines
	var transactions []dynamo.Transaction
	seen := map[string]bool{}
	for _, m := range totalRe.FindAllStringSubmatch(text, -1) {
		ticker := m[1]
		if seen[ticker] {
			continue
		}
		seen[ticker] = true

		qty, _ := strconv.Atoi(m[2])
		priceStr := strings.ReplaceAll(m[3], ",", ".")
		price, _ := strconv.ParseFloat(priceStr, 64)

		txType := cvByTicker[ticker]
		if txType == "" {
			txType = "C"
		}

		transactions = append(transactions, dynamo.Transaction{
			Ticker:      ticker,
			Date:        date,
			Quantity:    qty,
			Value:       price,
			Type:        txType,
			NotaNumber:  notaNumber,
		})
	}

	if len(transactions) == 0 {
		return nil, fmt.Errorf("no transactions found in nota")
	}
	return transactions, nil
}
