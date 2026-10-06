package guard

import (
	"bufio"
	"crypto/hmac"
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
// deletion is detectable by Verify. When the ledger has a key, each entry
// also carries an HMAC of its hash, so the chain cannot be rewritten and
// re-hashed by anyone who cannot read the key.
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
	MAC       string    `json:"mac,omitempty"`
}

// Ledger is a directory of daily JSONL files. Key, when set, keys the
// per-entry HMAC.
type Ledger struct {
	Dir string
	Key []byte
}

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

	sanitize(e)

	f, err := os.OpenFile(l.fileFor(e.Time), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("cannot lock ledger: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	prev, torn, err := lastHash(f)
	if err != nil {
		return err
	}

	e.PrevHash = prev
	e.Hash = hashEntry(e)
	e.MAC = l.mac(e.Hash)

	line, err := json.Marshal(e)
	if err != nil {
		return err
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	// A torn final line (crash or full disk mid-write) is left in place as
	// evidence but terminated, so this entry starts on its own line.
	if torn {
		line = append([]byte{'\n'}, line...)
	}

	_, err = f.Write(append(line, '\n'))

	return err
}

func (l Ledger) mac(hash string) string {
	if len(l.Key) == 0 {
		return ""
	}

	m := hmac.New(sha256.New, l.Key)
	m.Write([]byte(hash))

	return hex.EncodeToString(m.Sum(nil))
}

// sanitize makes every string valid UTF-8 so the hash computed here equals
// the one recomputed after a JSON round trip.
func sanitize(e *Entry) {
	for _, p := range []*string{&e.Agent, &e.SessionID, &e.Cwd, &e.Repo, &e.Event, &e.Tool, &e.Target, &e.Reason, &e.Outcome} {
		*p = strings.ToValidUTF8(*p, "�")
	}

	for _, list := range [][]string{e.Rules, e.Controls} {
		for i := range list {
			list[i] = strings.ToValidUTF8(list[i], "�")
		}
	}
}

func hashEntry(e *Entry) string {
	c := *e
	c.Hash = ""
	c.MAC = ""
	b, _ := json.Marshal(c)
	sum := sha256.Sum256(b)

	return hex.EncodeToString(sum[:])
}

// lastHash returns the hash of the last well-formed entry and whether the
// file ends in an unterminated line. Malformed lines are skipped here (so
// one bad line cannot stop all future appends) and reported by Verify.
func lastHash(f *os.File) (string, bool, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", false, err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}

	torn := len(data) > 0 && data[len(data)-1] != '\n'

	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var e Entry
		if json.Unmarshal([]byte(line), &e) == nil && e.Hash != "" {
			return e.Hash, torn, nil
		}
	}

	return "", torn, nil
}

// Recent returns up to limit entries, newest first, skipping malformed
// lines.
func (l Ledger) Recent(limit int) ([]Entry, error) {
	files, err := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}

	sort.Sort(sort.Reverse(sort.StringSlice(files)))

	var out []Entry
	for _, name := range files {
		entries, _, err := readEntries(name)
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

// ReadDay returns the well-formed entries of one daily file, oldest first.
func ReadDay(dir, day string) ([]Entry, error) {
	entries, _, err := readEntries(filepath.Join(dir, day+".jsonl"))
	return entries, err
}

// Verify checks every daily file and returns the number of verified
// entries, or the first problem found.
func (l Ledger) Verify() (int, error) {
	files, err := filepath.Glob(filepath.Join(l.Dir, "*.jsonl"))
	if err != nil {
		return 0, err
	}

	sort.Strings(files)

	n := 0
	for _, name := range files {
		day := strings.TrimSuffix(filepath.Base(name), ".jsonl")

		count, err := l.VerifyDay(day)
		n += count

		if err != nil {
			return n, err
		}
	}

	return n, nil
}

// VerifyDay checks one daily file: every line well-formed, each entry
// chained to the previous one, its hash correct, and (with a key) its MAC.
func (l Ledger) VerifyDay(day string) (int, error) {
	entries, bad, err := readEntries(filepath.Join(l.Dir, day+".jsonl"))
	if err != nil {
		return 0, err
	}

	if bad > 0 {
		return 0, fmt.Errorf("%s: %d malformed line(s)", day, bad)
	}

	prev := ""
	for i := range entries {
		e := &entries[i]

		if e.PrevHash != prev || hashEntry(e) != e.Hash {
			return i, fmt.Errorf("%s: chain broken at entry %d", day, i+1)
		}

		if len(l.Key) > 0 && !hmac.Equal([]byte(e.MAC), []byte(l.mac(e.Hash))) {
			return i, fmt.Errorf("%s: entry %d is not signed by this installation", day, i+1)
		}

		prev = e.Hash
	}

	return len(entries), nil
}

// readEntries returns the well-formed entries and the number of malformed
// lines.
func readEntries(name string) ([]Entry, int, error) {
	f, err := os.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}

	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	var (
		entries []Entry
		bad     int
	)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			bad++
			continue
		}

		entries = append(entries, e)
	}

	return entries, bad, sc.Err()
}
