package main

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"github.com/c0ze/armarium/internal/importer"
)

// libraryID resolves a configured library name. Run `armarium scan` first so the
// items the importer matches against exist.
func (a *app) libraryID(ctx context.Context, name string) (int64, error) {
	libs, err := a.store.Libraries(ctx)
	if err != nil {
		return 0, err
	}
	for _, l := range libs {
		if l.Name == name {
			return l.ID, nil
		}
	}
	return 0, fmt.Errorf("no library named %q in the config", name)
}

func (a *app) importSkrivist(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import-skrivist", flag.ExitOnError)
	db := fs.String("db", "", "path to reader_index.db")
	lib := fs.String("library", "", "Armarium library whose root holds the reader's *_downloads folders")
	fs.Parse(args)
	if *db == "" || *lib == "" {
		return errors.New("import-skrivist --db reader_index.db --library <name>")
	}
	id, err := a.libraryID(ctx, *lib)
	if err != nil {
		return err
	}
	res, err := importer.Skrivist(ctx, a.store, id, *db)
	if err == nil {
		fmt.Printf("progress rows: %d, tagged items: %d, unmatched: %d\n", res.Progress, res.Tags, res.Skipped)
	}
	return err
}

func (a *app) importKavita(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import-kavita", flag.ExitOnError)
	db := fs.String("db", "", "path to a copy of kavita.db")
	lib := fs.String("library", "", "Armarium library holding the same files")
	root := fs.String("root", "", "the library folder as Kavita saw it, e.g. /comics")
	fs.Parse(args)
	if *db == "" || *lib == "" || *root == "" {
		return errors.New("import-kavita --db kavita.db --library <name> --root <kavita path>")
	}
	id, err := a.libraryID(ctx, *lib)
	if err != nil {
		return err
	}
	res, err := importer.Kavita(ctx, a.store, id, *db, *root)
	if err == nil {
		fmt.Printf("progress rows: %d, unmatched: %d\n", res.Progress, res.Skipped)
	}
	return err
}
