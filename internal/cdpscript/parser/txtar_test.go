package parser

import (
	"bytes"
	"testing"
)

func TestParseTxtar(t *testing.T) {
	input := `This is a comment
-- file1.txt --
This is file 1 content
-- file2.txt --
This is file 2 content
More content
`

	archive, err := ParseTxtar([]byte(input))
	if err != nil {
		t.Fatalf("ParseTxtar failed: %v", err)
	}

	if archive.Comment != "This is a comment" {
		t.Errorf("expected comment 'This is a comment', got '%s'", archive.Comment)
	}

	if len(archive.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(archive.Files))
	}

	if archive.Files[0].Name != "file1.txt" {
		t.Errorf("file 0: expected name 'file1.txt', got '%s'", archive.Files[0].Name)
	}

	if archive.Files[1].Name != "file2.txt" {
		t.Errorf("file 1: expected name 'file2.txt', got '%s'", archive.Files[1].Name)
	}

	expectedContent1 := "This is file 1 content\n"
	if string(archive.Files[0].Data) != expectedContent1 {
		t.Errorf("file 0: expected content '%s', got '%s'", expectedContent1, string(archive.Files[0].Data))
	}
}

func TestTxtarGetFile(t *testing.T) {
	input := `-- test.txt --
test content
-- other.txt --
other content`

	archive, err := ParseTxtar([]byte(input))
	if err != nil {
		t.Fatalf("ParseTxtar failed: %v", err)
	}

	content, ok := archive.GetFile("test.txt")
	if !ok {
		t.Fatal("GetFile('test.txt') returned false")
	}

	expected := "test content\n"
	if string(content) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(content))
	}

	_, ok = archive.GetFile("nonexistent.txt")
	if ok {
		t.Error("GetFile('nonexistent.txt') should return false")
	}
}

func TestTxtarFormat(t *testing.T) {
	archive := &TxtarArchive{
		Comment: "Test archive",
		Files: []TxtarFile{
			{Name: "file1.txt", Data: []byte("content 1\n")},
			{Name: "file2.txt", Data: []byte("content 2\n")},
		},
	}

	var buf bytes.Buffer
	if err := archive.Format(&buf); err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// Parse it back
	parsed, err := ParseTxtar(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseTxtar failed: %v", err)
	}

	if parsed.Comment != archive.Comment {
		t.Errorf("comment mismatch: expected '%s', got '%s'", archive.Comment, parsed.Comment)
	}

	if len(parsed.Files) != len(archive.Files) {
		t.Fatalf("file count mismatch: expected %d, got %d", len(archive.Files), len(parsed.Files))
	}
}
