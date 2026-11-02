package storage

import (
	"io"
	"strings"
	"testing"
)

func TestSaveOpenRemove(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	if err := d.Save("p1/e1.jpg", strings.NewReader("photo")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f, err := d.Open("p1/e1.jpg")
	if err != nil {
		t.Fatalf("Open file: %v", err)
	}
	got, _ := io.ReadAll(f)
	_ = f.Close()
	if string(got) != "photo" {
		t.Fatalf("read %q", got)
	}
	if err := d.Remove("p1/e1.jpg"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := d.Remove("p1/e1.jpg"); err != nil {
		t.Fatalf("Remove twice: %v", err)
	}
}

func TestNamesCannotLeaveTheRoot(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Save("../escape.jpg", strings.NewReader("x")); err == nil {
		t.Fatal("saved outside the upload directory")
	}
}
