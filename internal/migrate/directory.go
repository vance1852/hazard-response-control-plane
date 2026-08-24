package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
)

func FromDirectory(ctx context.Context, db *sql.DB, directory string) error {
	info, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("stat migration directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("migration path %q is not a directory", directory)
	}
	migrations, err := Load(os.DirFS(directory))
	if err != nil {
		return err
	}
	return Apply(ctx, db, migrations)
}
