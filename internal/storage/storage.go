// Package storage keeps uploaded files on local disk under one directory. Every path
// goes through os.Root, so a stored name can never reach outside that directory.
package storage

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
)

// Disk stores files under a root directory.
type Disk struct {
	root *os.Root
}

// Open creates dir if needed and returns a Disk rooted there.
func Open(dir string) (*Disk, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open upload dir: %w", err)
	}
	return &Disk{root: root}, nil
}

// Close releases the root directory.
func (d *Disk) Close() error { return d.root.Close() }

// Save writes r to name ("project/expense.jpg"), replacing any file already there.
func (d *Disk) Save(name string, r io.Reader) error {
	if err := d.root.MkdirAll(path.Dir(name), 0o750); err != nil {
		return fmt.Errorf("create folder: %w", err)
	}
	f, err := d.root.Create(name)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close() // the copy already failed; the file is incomplete either way
		return fmt.Errorf("write file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}
	return nil
}

// Open returns the named file for reading.
func (d *Disk) Open(name string) (io.ReadSeekCloser, error) {
	f, err := d.root.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

// Remove deletes the named file; a missing file is not an error.
func (d *Disk) Remove(name string) error {
	if err := d.root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}
