package quotes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	// sqlite3
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

// Thresholds, it's in two different ones to avoid
// having to define as var and use sprintf
const (
	quoteThreshold    = -2
	quoteThresholdStr = "-2"
)

const (
	sqlCreateTable = `CREATE TABLE IF NOT EXISTS quotes (` +
		`id INTEGER PRIMARY KEY,` +
		`date INTEGER NOT NULL,` +
		`author TEXT NOT NULL,` +
		`quote TEXT NOT NULL);`
	sqlCreateVotesTable = `CREATE TABLE IF NOT EXISTS votes (` +
		`quote_id INTEGER NOT NULL,` +
		`voter TEXT NOT NULL,` +
		`vote INTEGER NOT NULL,` +
		`date INTEGER NOT NULL,` +
		`PRIMARY KEY (quote_id, voter),` +
		`FOREIGN KEY (quote_id) REFERENCES quotes (id))`
	sqlDateIndex        = `CREATE INDEX IF NOT EXISTS quotesdate ON quotes (date);`
	sqlVoteQuoteIDIndex = `CREATE INDEX IF NOT EXISTS quotesid ON votes (quote_id);`
	sqlVoteVoteIndex    = `CREATE INDEX IF NOT EXISTS votesvote ON votes (vote);`

	sqlGetCount = `SELECT COUNT(*) FROM quotes;`
	sqlAdd      = `INSERT INTO quotes (date, author, quote) VALUES(?, ?, ?);`
	sqlDel      = `DELETE FROM quotes WHERE id = ?;`
	sqlEdit     = `UPDATE quotes SET quote = ? WHERE id = ?;`

	sqlHasQuote = `SELECT EXISTS(SELECT id FROM quotes WHERE id = ?);`
	sqlGetByID  = `SELECT id, date, author, quote, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = 1) AS upvotes, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = -1) AS downvotes ` +
		`FROM quotes ` +
		`WHERE id = ?;`
	sqlGetRandom = `SELECT id, date, author, quote, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = 1) AS upvotes, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = -1) AS downvotes ` +
		`FROM quotes ` +
		`WHERE (upvotes - downvotes) > ` + quoteThresholdStr + ` ` +
		`ORDER BY RANDOM() LIMIT 1;`
	sqlGetAll = `SELECT q.id, q.date, q.author, q.quote, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = 1) AS upvotes, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = -1) AS downvotes ` +
		`FROM quotes as q ` +
		`ORDER BY q.id desc;`
	sqlGetAllFiltered = `SELECT q.id, q.date, q.author, q.quote, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = 1) AS upvotes, ` +
		`(SELECT COUNT(*) FROM votes WHERE quote_id = id AND vote = -1) AS downvotes ` +
		`FROM quotes as q ` +
		`WHERE (upvotes - downvotes) > ` + quoteThresholdStr + ` ` +
		`ORDER BY q.id desc;`

	sqlHasVote      = `SELECT vote FROM VOTES WHERE quote_id = ? AND voter = ? LIMIT 1;`
	sqlUpvote       = `INSERT INTO votes (quote_id, voter, vote, date) VALUES (?, ?, 1, ?);`
	sqlDownvote     = `INSERT INTO votes (quote_id, voter, vote, date) VALUES (?, ?, -1, ?);`
	sqlUnvote       = `DELETE FROM VOTES WHERE quote_id = ? AND voter = ?;`
	sqlGetUpvotes   = `SELECT COUNT(*) FROM votes WHERE quote_id = ? AND vote = 1;`
	sqlGetDownvotes = `SELECT COUNT(*) FROM votes WHERE quote_id = ? AND vote = -1;`
)

// QuoteDB provides file storage of quotes via an sqlite database.
type QuoteDB struct {
	db *sql.DB

	webuser string
	webpass string
	webhash []byte

	sync.RWMutex
	nQuotes int
}

// Quote is for serializing to and from the sqlite database.
type Quote struct {
	ID     int
	Date   time.Time
	Author string
	Quote  string

	Upvotes   int
	Downvotes int
}

// OpenDB opens the database at the location requested.
func OpenDB(filename, webAuth string) (*QuoteDB, error) {
	opts := make(url.Values)
	opts.Set("_foreign_keys", "1")

	var user, pass string
	var hash []byte
	if len(webAuth) != 0 {
		splits := strings.SplitN(webAuth, ":", 2)
		if len(splits) == 2 {
			user = splits[0]
			pass = splits[1]

			var err error
			hash, err = bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
			if err != nil {
				return nil, fmt.Errorf("failed to bcrypt web password: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite3", filename+`?`+opts.Encode())
	if err != nil {
		return nil, err
	}

	qdb := &QuoteDB{
		db:      db,
		webuser: user,
		webpass: pass,
		webhash: hash,
	}

	err = qdb.createTable()
	if err != nil {
		defer qdb.Close()
		return nil, err
	}
	err = qdb.getCount()
	if err != nil {
		defer qdb.Close()
		return nil, err
	}

	return qdb, nil
}

// NQuotes returns the number of quotes in the database.
func (q *QuoteDB) NQuotes() int {
	q.RLock()
	defer q.RUnlock()
	return q.nQuotes
}

// createTableIfNotExists creates the quotes table if necessary.
func (q *QuoteDB) createTable() (err error) {
	var commands = []string{
		sqlCreateTable,
		sqlCreateVotesTable,
		sqlDateIndex,
		sqlVoteQuoteIDIndex,
		sqlVoteVoteIndex,
	}

	for _, c := range commands {
		_, err = q.db.Exec(c)
		if err != nil {
			return fmt.Errorf("error running sql statement:\nsql: %s\nerror: %v", c, err)
		}
	}

	return nil
}

// getCount refreshes the number of quotes.
func (q *QuoteDB) getCount() error {
	return q.db.QueryRow(sqlGetCount).Scan(&q.nQuotes)
}

// Close the database file.
func (q *QuoteDB) Close() error {
	err := q.db.Close()
	q.db = nil
	return err
}

// AddQuote adds a quote to the database.
func (q *QuoteDB) AddQuote(author, quote string) (id int64, err error) {
	q.Lock()
	defer q.Unlock()

	var res sql.Result
	res, err = q.db.Exec(sqlAdd, time.Now().Unix(), author, quote)
	if err != nil {
		return
	}

	if id, err = res.LastInsertId(); err != nil {
		id = 0
	}

	q.nQuotes++
	return
}

// RandomQuote gets a random existing quote.
func (q *QuoteDB) RandomQuote() (quote Quote, err error) {
	var date int64
	err = q.db.QueryRow(sqlGetRandom).Scan(
		&quote.ID,
		&date,
		&quote.Author,
		&quote.Quote,
		&quote.Upvotes,
		&quote.Downvotes)
	if err != nil {
		return quote, err
	}

	quote.Date = time.Unix(date, 0).UTC()

	return quote, err
}

// GetQuote gets a specific quote by id.
func (q *QuoteDB) GetQuote(id int) (quote Quote, err error) {
	var date int64
	err = q.db.QueryRow(sqlGetByID, id).Scan(
		&quote.ID,
		&date,
		&quote.Author,
		&quote.Quote,
		&quote.Upvotes,
		&quote.Downvotes)
	if err != nil {
		return quote, err
	}

	quote.Date = time.Unix(date, 0).UTC()

	return quote, nil
}

// DelQuote deletes a quote by id.
func (q *QuoteDB) DelQuote(id int) (bool, error) {
	var err error
	var res sql.Result
	var r int64
	if res, err = q.db.Exec(sqlDel, id); err != nil {
		return false, err
	}
	if r, err = res.RowsAffected(); err != nil {
		return false, err
	}
	if r == 1 {
		q.Lock()
		defer q.Unlock()
		q.nQuotes--
		return true, nil
	}
	return false, nil
}

// EditQuote edits a quote by id.
func (q *QuoteDB) EditQuote(id int, quote string) (bool, error) {
	var err error
	var res sql.Result
	var r int64
	if res, err = q.db.Exec(sqlEdit, quote, id); err != nil {
		return false, err
	}
	if r, err = res.RowsAffected(); err != nil {
		return false, err
	}
	return r == 1, nil
}

// GetAll quotes
func (q *QuoteDB) GetAll(filterLow bool) ([]Quote, error) {
	var err error

	query := sqlGetAll
	if filterLow {
		query = sqlGetAllFiltered
	}
	rows, err := q.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	quotes := make([]Quote, 0)
	quote := Quote{}
	for rows.Next() {
		var date int64
		if err = rows.Scan(&quote.ID, &date, &quote.Author, &quote.Quote, &quote.Upvotes, &quote.Downvotes); err != nil {
			return nil, err
		}

		quote.Date = time.Unix(date, 0).UTC()

		quotes = append(quotes, quote)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return quotes, nil
}

// Upvote returns true iff the upvote was applied, if it was not applied
// it's because the user already has a vote for that quote
func (q *QuoteDB) Upvote(id int, voter string) (bool, error) {
	tx, err := q.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return false, err
	}

	// If we have a +1 already, return false, nil
	// If we have a -1, delete it, and add the +1
	// If we have nothing, add the +1
	var quoteExists int
	err = tx.QueryRow(sqlHasQuote, id).Scan(&quoteExists)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}

	if quoteExists == 0 {
		_ = tx.Rollback()
		return false, errors.New("Not a valid id")
	}

	var vote int
	err = tx.QueryRow(sqlHasVote, id, voter).Scan(&vote)
	if err != nil && err != sql.ErrNoRows {
		_ = tx.Rollback()
		return false, nil
	}

	switch {
	case vote > 0:
		// Return false, we've already got the same type of vote here
		_ = tx.Rollback()
		return false, nil
	case vote < 0:
		// Delete old downvote
		if _, err = tx.Exec(sqlUnvote, id, voter); err != nil {
			_ = tx.Rollback()
			return false, err
		}
	}

	if _, err = tx.Exec(sqlUpvote, id, voter, time.Now().Unix()); err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return true, nil
}

// Downvote returns true iff the upvote was applied, if it was not applied
// it's because the user already has a vote for that quote
func (q *QuoteDB) Downvote(id int, voter string) (bool, error) {
	tx, err := q.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return false, err
	}

	// If we have a -1 already, return false, nil
	// If we have a +1, delete it, and add the -1
	// If we have nothing, add the -1
	var quoteExists int
	err = tx.QueryRow(sqlHasQuote, id).Scan(&quoteExists)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}

	if quoteExists == 0 {
		_ = tx.Rollback()
		return false, errors.New("Not a valid id")
	}

	var vote int
	err = tx.QueryRow(sqlHasVote, id, voter).Scan(&vote)
	if err != nil && err != sql.ErrNoRows {
		_ = tx.Rollback()
		return false, nil
	}

	switch {
	case vote < 0:
		// Return false, we've already got the same type of vote here
		_ = tx.Rollback()
		return false, nil
	case vote > 0:
		// Delete old upvote
		if _, err = tx.Exec(sqlUnvote, id, voter); err != nil {
			_ = tx.Rollback()
			return false, err
		}
	}

	if _, err = tx.Exec(sqlDownvote, id, voter, time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return true, nil
}

// Unvote returns true iff there was a vote that was removed, otherwise it
// return false.
func (q *QuoteDB) Unvote(id int, voter string) (bool, error) {
	tx, err := q.db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: false})
	if err != nil {
		return false, err
	}

	var quoteExists int
	err = tx.QueryRow(sqlHasQuote, id).Scan(&quoteExists)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}

	if quoteExists == 0 {
		_ = tx.Rollback()
		return false, errors.New("Not a valid id")
	}

	var throwaway int
	err = tx.QueryRow(sqlHasVote, id, voter).Scan(&throwaway)
	if err == sql.ErrNoRows {
		_ = tx.Rollback()
		return false, nil
	} else if err != nil {
		_ = tx.Rollback()
		return false, err
	}

	if _, err = tx.Exec(sqlUnvote, id, voter); err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return true, nil
}

// Votes retrieves the vote counts for a quote
func (q *QuoteDB) Votes(id int) (up, down int, err error) {
	if err = q.db.QueryRow(sqlGetUpvotes, id).Scan(&up); err != nil {
		return 0, 0, err
	}
	if err = q.db.QueryRow(sqlGetUpvotes, id).Scan(&down); err != nil {
		return 0, 0, err
	}

	return up, down, nil
}
