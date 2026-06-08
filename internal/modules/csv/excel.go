package csv

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// SheetNames opens an .xlsx workbook and returns its sheet names in workbook
// order. The bytes come through the fs seam so the same atomic-read path
// serves CSV and Excel. A non-workbook or unreadable file surfaces an error.
func (m *Manager) SheetNames(filePath string) ([]string, error) {
	f, err := m.openWorkbook(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.GetSheetList(), nil
}

// PreviewSheet reads one sheet of an .xlsx workbook into the same PreviewResult
// shape Preview returns for CSV, so the downstream mapping pipeline is identical
// regardless of source. The first row is the header; cells are read formatted
// (GetCellValue applies the cell's number/date format), so date columns come
// through readable instead of as serial numbers. An empty sheet name falls back
// to the workbook's active sheet.
func (m *Manager) PreviewSheet(filePath, sheet string) (PreviewResult, error) {
	f, err := m.openWorkbook(filePath)
	if err != nil {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}, Error: err.Error()}, err
	}
	defer f.Close()

	if sheet == "" {
		if idx := f.GetActiveSheetIndex(); idx >= 0 {
			sheet = f.GetSheetName(idx)
		}
	}
	records, err := f.GetRows(sheet)
	if err != nil {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}, Error: err.Error()},
			fmt.Errorf("csv: read sheet %q in %q: %w", sheet, filePath, err)
	}
	if len(records) == 0 {
		return PreviewResult{Headers: []string{}, Rows: [][]string{}}, nil
	}

	headers := records[0]
	rows := records[1:]
	if rows == nil {
		rows = [][]string{}
	}
	return PreviewResult{Headers: headers, Rows: rows, RowCount: len(rows)}, nil
}

// WriteExcel serializes rows (first row = headers, then data) to an .xlsx file
// with one sheet. An empty sheet name falls back to "Sheet1". Cells are written
// as strings (the export side already formatted every value), matching CSV
// Write's plain-text contract. The bytes go through the fs seam's atomic write.
func (m *Manager) WriteExcel(filePath string, rows [][]string, sheet string) WriteResult {
	if sheet == "" {
		sheet = "Sheet1"
	}
	f := excelize.NewFile()
	defer f.Close()
	// NewFile seeds a "Sheet1"; rename it to the requested sheet so there is
	// exactly one sheet with the right name.
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return WriteResult{Error: err.Error()}
	}
	for r, row := range rows {
		for c, val := range row {
			cell, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				return WriteResult{Error: err.Error()}
			}
			// SetCellStr keeps GUIDs and numeric-looking ids as text instead of
			// coercing them to numbers/dates.
			if err := f.SetCellStr(sheet, cell, val); err != nil {
				return WriteResult{Error: err.Error()}
			}
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return WriteResult{Error: err.Error()}
	}
	if err := m.fs.SaveBytes(filePath, buf.Bytes()); err != nil {
		return WriteResult{Error: err.Error()}
	}
	return WriteResult{Success: true}
}

// openWorkbook reads the file through the fs seam and parses it as xlsx. The
// caller must Close the returned file.
func (m *Manager) openWorkbook(filePath string) (*excelize.File, error) {
	raw, err := m.fs.LoadBytes(filePath)
	if err != nil {
		return nil, fmt.Errorf("csv: read %q: %w", filePath, err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("csv: open workbook %q: %w", filePath, err)
	}
	return f, nil
}
