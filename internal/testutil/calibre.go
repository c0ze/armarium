package testutil

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// CalibreBook is one book for WriteCalibre. Files maps an upper-case Calibre
// format ("EPUB", "MOBI") to the file's contents.
type CalibreBook struct {
	Title, Author, Series string
	SeriesIndex           float64
	Tags                  []string
	Cover                 []byte // written as cover.jpg when set
	Files                 map[string][]byte
}

// WriteCalibre builds a Calibre library under root: metadata.db with the
// tables Armarium reads, plus Author/Title (id)/ folders holding the files.
func WriteCalibre(t testing.TB, root string, books ...CalibreBook) {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(root, "metadata.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	for _, q := range []string{
		`CREATE TABLE books (id INTEGER PRIMARY KEY, title TEXT, sort TEXT, timestamp TEXT, pubdate TEXT, series_index REAL DEFAULT 1.0,
		  author_sort TEXT, isbn TEXT, lccn TEXT, path TEXT NOT NULL DEFAULT '', flags INTEGER, uuid TEXT, has_cover BOOL DEFAULT 0, last_modified TEXT)`,
		`CREATE TABLE authors (id INTEGER PRIMARY KEY, name TEXT, sort TEXT, link TEXT)`,
		`CREATE TABLE books_authors_link (id INTEGER PRIMARY KEY, book INTEGER, author INTEGER)`,
		`CREATE TABLE series (id INTEGER PRIMARY KEY, name TEXT, sort TEXT)`,
		`CREATE TABLE books_series_link (id INTEGER PRIMARY KEY, book INTEGER, series INTEGER)`,
		`CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT)`,
		`CREATE TABLE books_tags_link (id INTEGER PRIMARY KEY, book INTEGER, tag INTEGER)`,
		`CREATE TABLE data (id INTEGER PRIMARY KEY, book INTEGER, format TEXT, uncompressed_size INTEGER, name TEXT)`,
	} {
		exec(q)
	}
	for i, b := range books {
		id := int64(i + 1)
		dir := filepath.Join(b.Author, b.Title+" ("+itoa(id)+")")
		exec(`INSERT INTO books (id, title, sort, path, has_cover, series_index) VALUES (?, ?, ?, ?, ?, ?)`,
			id, b.Title, b.Title, filepath.ToSlash(dir), b.Cover != nil, b.SeriesIndex)
		exec(`INSERT OR IGNORE INTO authors (name, sort) VALUES (?, ?)`, b.Author, b.Author)
		exec(`INSERT INTO books_authors_link (book, author) SELECT ?, id FROM authors WHERE name = ?`, id, b.Author)
		if b.Series != "" {
			exec(`INSERT INTO series (name, sort) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM series WHERE name = ?)`, b.Series, b.Series, b.Series)
			exec(`INSERT INTO books_series_link (book, series) SELECT ?, id FROM series WHERE name = ?`, id, b.Series)
		}
		for _, tag := range b.Tags {
			exec(`INSERT INTO tags (name) SELECT ? WHERE NOT EXISTS (SELECT 1 FROM tags WHERE name = ?)`, tag, tag)
			exec(`INSERT INTO books_tags_link (book, tag) SELECT ?, id FROM tags WHERE name = ?`, id, tag)
		}
		os.MkdirAll(filepath.Join(root, dir), 0o755)
		if b.Cover != nil {
			os.WriteFile(filepath.Join(root, dir, "cover.jpg"), b.Cover, 0o644)
		}
		name := b.Title + " - " + b.Author
		for format, data := range b.Files {
			exec(`INSERT INTO data (book, format, uncompressed_size, name) VALUES (?, ?, ?, ?)`, id, format, len(data), name)
			ext := map[string]string{"EPUB": "epub", "MOBI": "mobi", "PDF": "pdf", "CBZ": "cbz", "AZW3": "azw3", "TXT": "txt"}[format]
			os.WriteFile(filepath.Join(root, dir, name+"."+ext), data, 0o644)
		}
	}
}

func itoa(i int64) string {
	b := []byte{}
	for {
		b = append([]byte{byte('0' + i%10)}, b...)
		if i /= 10; i == 0 {
			return string(b)
		}
	}
}
