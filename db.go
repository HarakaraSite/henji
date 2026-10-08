package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
)

var (
	errNoMatches   = errors.New("no conversations found")
	errManyMatches = errors.New("multiple conversations matched the input")
)

func handleSqliteErr(err error) error {
	sqerr := &sqlite.Error{}
	if errors.As(err, &sqerr) {
		return fmt.Errorf(
			"%w: %s",
			sqerr,
			sqlite.ErrorCodeString[sqerr.Code()],
		)
	}
	return err
}

func openDB(ds string) (*convoDB, error) {
	// Apply the timeout to every connection, including connections opened later
	// by the pool. Serialize schema changes across simultaneous CLI startups.
	separator := "?"
	if strings.Contains(ds, "?") {
		separator = "&"
	}
	db, err := sqlx.Open("sqlite", ds+separator+"_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf(
			"could not create db: %w",
			handleSqliteErr(err),
		)
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = db.Close()
		}
	}()
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf(
			"could not ping db: %w",
			handleSqliteErr(err),
		)
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("could not begin db migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		CREATE TABLE
		  IF NOT EXISTS conversations (
		    id string NOT NULL PRIMARY KEY,
		    title string NOT NULL,
		    updated_at datetime NOT NULL DEFAULT (strftime ('%Y-%m-%d %H:%M:%f', 'now')),
		    CHECK (id <> ''),
		    CHECK (title <> '')
		  )
	`); err != nil {
		return nil, fmt.Errorf("could not migrate db: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_conv_id ON conversations (id)
	`); err != nil {
		return nil, fmt.Errorf("could not migrate db: %w", err)
	}
	if _, err := tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_conv_title ON conversations (title)
	`); err != nil {
		return nil, fmt.Errorf("could not migrate db: %w", err)
	}

	if !hasColumn(tx, "model") {
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN model string
		`); err != nil {
			return nil, fmt.Errorf("could not migrate db: %w", err)
		}
	}
	if !hasColumn(tx, "api") {
		if _, err := tx.Exec(`
			ALTER TABLE conversations ADD COLUMN api string
		`); err != nil {
			return nil, fmt.Errorf("could not migrate db: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit db migration: %w", err)
	}
	initialized = true
	return &convoDB{db: db}, nil
}

func hasColumn(tx *sqlx.Tx, col string) bool {
	var count int
	if err := tx.Get(&count, `
		SELECT count(*)
		FROM pragma_table_info('conversations') c
		WHERE c.name = $1
	`, col); err != nil {
		return false
	}
	return count > 0
}

type convoDB struct {
	db *sqlx.DB
}

// Conversation in the database.
type Conversation struct {
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	UpdatedAt time.Time `db:"updated_at"`
	API       *string   `db:"api"`
	Model     *string   `db:"model"`
}

func (c *convoDB) Close() error {
	return c.db.Close() //nolint: wrapcheck
}

func (c *convoDB) Save(id, title, api, model string) error {
	res, err := c.db.Exec(c.db.Rebind(`
		UPDATE conversations
		SET
		  title = ?,
		  api = ?,
		  model = ?,
		  updated_at = CURRENT_TIMESTAMP
		WHERE
		  id = ?
	`), title, api, model, id)
	if err != nil {
		return fmt.Errorf("Save: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("Save: %w", err)
	}

	if rows > 0 {
		return nil
	}

	if _, err := c.db.Exec(c.db.Rebind(`
		INSERT INTO
		  conversations (id, title, api, model)
		VALUES
		  (?, ?, ?, ?)
	`), id, title, api, model); err != nil {
		return fmt.Errorf("Save: %w", err)
	}

	return nil
}

func (c *convoDB) Delete(id string) error {
	if _, err := c.db.Exec(c.db.Rebind(`
		DELETE FROM conversations
		WHERE
		  id = ?
	`), id); err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

func (c *convoDB) FindHEAD() (*Conversation, error) {
	var convo Conversation
	if err := c.db.Get(&convo, `
		SELECT
		  *
		FROM
		  conversations
		ORDER BY
		  updated_at DESC
		LIMIT
		  1
	`); err != nil {
		return nil, fmt.Errorf("FindHead: %w", err)
	}
	return &convo, nil
}

func (c *convoDB) findByExactTitle(result *[]Conversation, in string) error {
	if err := c.db.Select(result, c.db.Rebind(`
		SELECT
		  *
		FROM
		  conversations
		WHERE
		  title = ?
	`), in); err != nil {
		return fmt.Errorf("findByExactTitle: %w", err)
	}
	return nil
}

func (c *convoDB) findByIDOrTitle(result *[]Conversation, in string) error {
	if err := c.db.Select(result, c.db.Rebind(`
		SELECT
		  *
		FROM
		  conversations
		WHERE
		  id glob ?
		  OR title = ?
	`), in+"*", in); err != nil {
		return fmt.Errorf("findByIDOrTitle: %w", err)
	}
	return nil
}

func (c *convoDB) Completions(in string) ([]string, error) {
	var result []string
	if err := c.db.Select(&result, c.db.Rebind(`
		SELECT
		  printf (
		    '%s%c%s',
		    CASE
		      WHEN length (?) < ? THEN substr (id, 1, ?)
		      ELSE id
		    END,
		    char(9),
		    title
		  )
		FROM
		  conversations
		WHERE
		  id glob ?
		UNION
		SELECT
		  printf ("%s%c%s", title, char(9), substr (id, 1, ?))
		FROM
		  conversations
		WHERE
		  title glob ?
	`), in, sha1short, sha1short, in+"*", sha1short, in+"*"); err != nil {
		return result, fmt.Errorf("Completions: %w", err)
	}
	return result, nil
}

func (c *convoDB) Find(in string) (*Conversation, error) {
	var conversations []Conversation
	var err error

	if len(in) < sha1minLen {
		err = c.findByExactTitle(&conversations, in)
	} else {
		err = c.findByIDOrTitle(&conversations, in)
	}
	if err != nil {
		return nil, fmt.Errorf("Find %q: %w", in, err)
	}

	if len(conversations) > 1 {
		return nil, fmt.Errorf("%w: %s", errManyMatches, in)
	}
	if len(conversations) == 1 {
		return &conversations[0], nil
	}
	return nil, fmt.Errorf("%w: %s", errNoMatches, in)
}

func (c *convoDB) List() ([]Conversation, error) {
	var convos []Conversation
	if err := c.db.Select(&convos, `
		SELECT
		  *
		FROM
		  conversations
		ORDER BY
		  updated_at DESC
	`); err != nil {
		return convos, fmt.Errorf("List: %w", err)
	}
	return convos, nil
}
