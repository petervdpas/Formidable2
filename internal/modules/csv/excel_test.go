package csv

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"
)

// bytesFS serves fixed bytes for LoadBytes; LoadFile/SaveFile are unused here.
type bytesFS struct {
	data []byte
	err  error
}

func (b *bytesFS) LoadFile(string) (string, error) { return "", nil }
func (b *bytesFS) LoadBytes(string) ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.data, nil
}
func (b *bytesFS) SaveFile(string, string) error { return nil }

// buildWorkbook returns the bytes of a small .xlsx with one data sheet and a
// blank second sheet, including a date-formatted cell.
func buildWorkbook(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	const sheet = "Data"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		t.Fatal(err)
	}
	f.SetActiveSheet(idx)
	_ = f.SetSheetName("Sheet1", "Empty")

	f.SetCellValue(sheet, "A1", "id")
	f.SetCellValue(sheet, "B1", "naam")
	f.SetCellValue(sheet, "C1", "datum")
	f.SetCellValue(sheet, "A2", "guid-1")
	f.SetCellValue(sheet, "B2", "Aanbestedingssysteem")
	// A real date cell with a date number format, so GetCellValue formats it
	// instead of returning the serial.
	style, err := f.NewStyle(&excelize.Style{NumFmt: 14}) // m/d/yy
	if err != nil {
		t.Fatal(err)
	}
	f.SetCellValue(sheet, "C2", 45123) // serial -> a real date
	f.SetCellStyle(sheet, "C2", "C2", style)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestSheetNames(t *testing.T) {
	m := NewManager(&bytesFS{data: buildWorkbook(t)}, nil)
	names, err := m.SheetNames("book.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("want 2 sheets, got %v", names)
	}
	// "Data" must be present.
	found := false
	for _, n := range names {
		if n == "Data" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'Data' sheet, got %v", names)
	}
}

func TestPreviewSheet_HeadersRowsAndDate(t *testing.T) {
	m := NewManager(&bytesFS{data: buildWorkbook(t)}, nil)
	pr, err := m.PreviewSheet("book.xlsx", "Data")
	if err != nil {
		t.Fatal(err)
	}
	if got := pr.Headers; len(got) != 3 || got[0] != "id" || got[2] != "datum" {
		t.Fatalf("headers wrong: %v", got)
	}
	if pr.RowCount != 1 || len(pr.Rows) != 1 {
		t.Fatalf("want 1 data row, got %d", pr.RowCount)
	}
	row := pr.Rows[0]
	if row[0] != "guid-1" || row[1] != "Aanbestedingssysteem" {
		t.Errorf("row values wrong: %v", row)
	}
	// The date cell must be formatted, not the raw serial "45123".
	if row[2] == "45123" || row[2] == "" {
		t.Errorf("date cell not formatted, got %q", row[2])
	}
}

func TestPreviewSheet_ActiveSheetWhenEmptyName(t *testing.T) {
	m := NewManager(&bytesFS{data: buildWorkbook(t)}, nil)
	pr, err := m.PreviewSheet("book.xlsx", "")
	if err != nil {
		t.Fatal(err)
	}
	// Active sheet is "Data".
	if pr.RowCount != 1 {
		t.Fatalf("want active sheet 'Data' with 1 row, got %d", pr.RowCount)
	}
}

func TestPreviewSheet_MissingSheet(t *testing.T) {
	m := NewManager(&bytesFS{data: buildWorkbook(t)}, nil)
	pr, err := m.PreviewSheet("book.xlsx", "Nope")
	if err == nil {
		t.Fatal("expected error for missing sheet")
	}
	if pr.Error == "" {
		t.Errorf("expected PreviewResult.Error set, got %+v", pr)
	}
}

func TestPreviewSheet_EmptySheet(t *testing.T) {
	m := NewManager(&bytesFS{data: buildWorkbook(t)}, nil)
	pr, err := m.PreviewSheet("book.xlsx", "Empty")
	if err != nil {
		t.Fatal(err)
	}
	if pr.RowCount != 0 || len(pr.Headers) != 0 {
		t.Errorf("want empty result for empty sheet, got %+v", pr)
	}
}

func TestSheetNames_NotAWorkbook(t *testing.T) {
	m := NewManager(&bytesFS{data: []byte("this is not xlsx")}, nil)
	if _, err := m.SheetNames("bad.xlsx"); err == nil {
		t.Fatal("expected error opening a non-workbook")
	}
}

func TestPreviewSheet_LoadError(t *testing.T) {
	m := NewManager(&bytesFS{err: errLoadBoom}, nil)
	pr, err := m.PreviewSheet("x.xlsx", "Data")
	if err == nil {
		t.Fatal("expected load error")
	}
	if pr.Error == "" {
		t.Errorf("expected PreviewResult.Error set, got %+v", pr)
	}
}
