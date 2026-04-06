package mcpserver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type minimaxXlsxInput struct {
	FileName           string     `json:"file_name" jsonschema:"Extension hint for output file, e.g. report.xlsx. System generates a unique filename."`
	SheetName          string     `json:"sheet_name,omitempty" jsonschema:"Worksheet name, default Sheet1"`
	Headers            []string   `json:"headers,omitempty" jsonschema:"Header row"`
	Rows               [][]string `json:"rows,omitempty" jsonschema:"Data rows"`
	IncludeCurrentTime bool       `json:"include_current_time,omitempty" jsonschema:"Append current time row, default true"`
	Overwrite          bool       `json:"overwrite,omitempty" jsonschema:"Overwrite existing file, default false"`
}

type minimaxXlsxOutput struct {
	Path      string `json:"path" jsonschema:"Absolute path of written xlsx file"`
	URL       string `json:"url" jsonschema:"HTTP URL to access the file"`
	Filename  string `json:"filename" jsonschema:"Generated filename"`
	SizeBytes int    `json:"size_bytes" jsonschema:"Output file size in bytes"`
	Created   bool   `json:"created" jsonschema:"Whether file was newly created"`
	Rows      int    `json:"rows" jsonschema:"Total data rows excluding header"`
	Columns   int    `json:"columns" jsonschema:"Column count"`
}

func minimaxXlsx(_ context.Context, _ *mcp.CallToolRequest, in minimaxXlsxInput) (*mcp.CallToolResult, minimaxXlsxOutput, error) {
	out, err := minimaxXlsxToDisk(in)
	if err != nil {
		return nil, minimaxXlsxOutput{}, err
	}
	return nil, out, nil
}

func minimaxXlsxToDisk(in minimaxXlsxInput) (minimaxXlsxOutput, error) {
	fileName := strings.TrimSpace(in.FileName)
	if fileName == "" {
		fileName = "report.xlsx"
	}
	if !strings.HasSuffix(strings.ToLower(fileName), ".xlsx") {
		fileName += ".xlsx"
	}

	sheetName := strings.TrimSpace(in.SheetName)
	if sheetName == "" {
		sheetName = "Sheet1"
	}
	includeTime := true
	if in.IncludeCurrentTime == false {
		includeTime = false
	}

	headers := append([]string(nil), in.Headers...)
	rows := make([][]string, 0, len(in.Rows))
	maxCols := len(headers)
	for _, row := range in.Rows {
		r := append([]string(nil), row...)
		if len(r) > maxCols {
			maxCols = len(r)
		}
		rows = append(rows, r)
	}

	now, _ := getSystemTimeOutput()
	if includeTime {
		timeRow := []string{"Current Time", now.NowLocal, now.NowRFC3339, strconv.FormatInt(now.NowUnix, 10), now.TimezoneName}
		rows = append(rows, timeRow)
		if len(timeRow) > maxCols {
			maxCols = len(timeRow)
		}
	}
	if maxCols == 0 {
		maxCols = 3
	}
	if len(headers) == 0 {
		headers = make([]string, 0, maxCols)
		for i := 1; i <= maxCols; i++ {
			headers = append(headers, fmt.Sprintf("Column%d", i))
		}
	}
	if len(headers) > maxCols {
		maxCols = len(headers)
	}
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], "")
		}
	}

	xlsxData, err := buildMinimalXLSX(sheetName, headers, rows)
	if err != nil {
		return minimaxXlsxOutput{}, err
	}
	wrote, err := writeFrontendTempBytes(fileName, xlsxData, in.Overwrite)
	if err != nil {
		return minimaxXlsxOutput{}, err
	}
	return minimaxXlsxOutput{
		Path:      wrote.Path,
		URL:       wrote.URL,
		Filename:  wrote.Filename,
		SizeBytes: wrote.SizeBytes,
		Created:   wrote.Created,
		Rows:      len(rows),
		Columns:   maxCols,
	}, nil
}

func buildMinimalXLSX(sheetName string, headers []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	entries := map[string]string{
		"[Content_Types].xml":        contentTypesXML(),
		"_rels/.rels":                rootRelsXML(),
		"docProps/app.xml":           appXML(),
		"docProps/core.xml":          coreXML(),
		"xl/workbook.xml":            workbookXML(sheetName),
		"xl/_rels/workbook.xml.rels": workbookRelsXML(),
		"xl/styles.xml":              stylesXML(),
		"xl/worksheets/sheet1.xml":   worksheetXML(headers, rows),
	}

	order := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"docProps/app.xml",
		"docProps/core.xml",
		"xl/workbook.xml",
		"xl/_rels/workbook.xml.rels",
		"xl/styles.xml",
		"xl/worksheets/sheet1.xml",
	}
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := w.Write([]byte(entries[name])); err != nil {
			return nil, fmt.Errorf("write zip entry %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zip writer: %w", err)
	}
	return buf.Bytes(), nil
}

func worksheetXML(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	writeRow := func(rowNum int, cols []string) {
		b.WriteString(`<row r="`)
		b.WriteString(strconv.Itoa(rowNum))
		b.WriteString(`">`)
		for i, cell := range cols {
			ref := colName(i + 1)
			b.WriteString(`<c r="`)
			b.WriteString(ref)
			b.WriteString(strconv.Itoa(rowNum))
			b.WriteString(`" t="inlineStr"><is><t>`)
			b.WriteString(xmlEscape(cell))
			b.WriteString(`</t></is></c>`)
		}
		b.WriteString(`</row>`)
	}
	writeRow(1, headers)
	for i, row := range rows {
		writeRow(i+2, row)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func colName(n int) string {
	if n <= 0 {
		return "A"
	}
	var out []byte
	for n > 0 {
		n--
		out = append([]byte{byte('A' + (n % 26))}, out...)
		n /= 26
	}
	return string(out)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`
}

func rootRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`
}

func appXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>MiniMax Skill MCP</Application>
</Properties>`
}

func coreXML() string {
	now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:creator>MiniMax Skill</dc:creator>
  <cp:lastModifiedBy>MiniMax Skill</cp:lastModifiedBy>
  <dcterms:created xsi:type="dcterms:W3CDTF">` + now + `</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">` + now + `</dcterms:modified>
</cp:coreProperties>`
}

func workbookXML(sheetName string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="` + xmlEscape(sheetName) + `" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`
}

func workbookRelsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`
}

func stylesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>
  <fills count="1"><fill><patternFill patternType="none"/></fill></fills>
  <borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>
  <cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>
  <cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>
  <cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>
</styleSheet>`
}
