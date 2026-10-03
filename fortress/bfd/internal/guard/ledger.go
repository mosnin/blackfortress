package guard

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// Entry is one ledger record. Entries are hash-chained: each carries the
// hash of the previous entry in the same daily file, so any edit or
// deletion is detectable by Verify.
type Entry struct {
	Time      time.Time `json:"time"`
	Agent     string    `json:"agent"`
	SessionID string    `json:"session_id,omitempty"`
	Cwd       string    `json:"cwd,omitempty"`
	Repo      string    `json:"repo,omitempty"`
	Event     string    `json:"event"`
	Tool      string    `json:"tool,omitempty"`
	Target    string    `json:"target,omitempty"`
	Decision  Action    `json:"decision,omitempty"`
	Rules     []string  `json:"rules,omitempty"`
	Controls  []string  `json:"controls,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	Outcome   string    `json:"outcome,omitempty"`
	PrevHash  string    `json:"prev_hash"`
	Hash      string    `json:"hash"`
}

type Ledger struct{ Dir string }

func (l Ledger) fileFor(t time.Time) string {
	return filepath.Join(l.Dir, t.UTC().Format("2006-01-02")+".jsonl")
}

// Append writes e to today's file, filling in the hash chain. It takes an
// exclusive file lock so concurrent hook processes keep the chain intact.
func (l Ledger) Append(e *Entry) error {
	if err := os.MkdirAll(l.Dir, 0o700); err != nil {
		return err
	}

	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}

	f, err := os.OpenFile(l.fileFor(e.Time), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("cannot lock ledger: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	prev, err := lastHash(f)
	if err != nil {
		return err
	}

	e.PrevHash = prev
	e.Hash = ""
	e.Hash = hashEntry(e)

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	_, err = f.Write(append(line, '\n'))

	return err
}

func hashEntry(e *Entry) string {
	c := *e
	c.Hash = ""
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

func lastHash(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	var last string

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			last = line
		}
	}

	if err := sc.Err(); err != nil {
		return "", err
	}

	if last == "" {
		return "", nil
	}

	var e Entry
	if err := json.Unmarshal([]byte(last), &e); err != nil {
		return "", fmt.Errorf("corrupt ledger tail: %w", err)
	}

	return e.Hash, nil
}

// Recent returns up to limit entries, newest first.
func (l Ledger) Recent(limit int) ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	var out []Entry
	for _, name := range files {
		entries, err := readEntries(name)
		if err != nil {
			return nil, err
		}

		for i := len(entries) - 1; i >= 0 && len(out) < limit; i-- {
			out = append(out, entries[i])
		}

		if len(out) >= limit {
			break
		}
	}

	return out, nil
}

// Verify checks every daily file's hash chain and returns the first break.
func (l Ledger) Verify() (int, error) {
	files, err := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))
	if err != nil {
		return 0, err
	}

	sort.Strings(files)

	n := 0
	for _, name := range files {
		entries, err := readEntries(name)
		if err != nil {
			return n, err
		}

		prev := ""
		for i := range entries {
			e := &entries[i]
			if e.PrevHash != prev || hashEntry(e) != e.Hash {
				return n, fmt.Errorf("%s: chain broken at entry %d", filepath.Base(name), i+1)
			}

			prev = e.Hash
			n++
		}
	}

	return n, nil
}

func readEntries(name string) ([]Entry, error) {
	f, err := os.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(name), err)
		}

		entries = append(entries, e)
	}

	return entries, sc.Err()
}

// ReadDay returns the entries of one daily file (YYYY-MM-DD), oldest first.
func ReadDay(dir, day string) ([]Entry, error) {
	return readEntries(filepath.Join(dir, day+".jsonl"))
}

// VerifyDay checks one daily file's hash chain.
func VerifyDay(dir, day string) (int, error) {
	entries, err := ReadDay(dir, day)
	if err != nil {
		return 0, err
	}

	prev := ""
	for i := range entries {
		e := &entries[i]
		if e.PrevHash != prev || hashEntry(e) != e.Hash {
			return i, fmt.Errorf("%s: chain broken at entry %d", day, i+1)
		}

		prev = e.Hash
	}

	return len(entries), nil
}
