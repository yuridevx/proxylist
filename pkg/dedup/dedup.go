// deduplicator.go
package dedup

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"time"

	"github.com/yuridevx/proxylist/domain"
	bolt "go.etcd.io/bbolt"
)

// Deduplicator holds the bbolt DB.
type Deduplicator struct {
	db     *bolt.DB
	bucket []byte
}

// NewDefault opens (or creates) a file named "db.bolt" in the same folder as the executable.
func NewDefault() (*Deduplicator, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(exePath)
	dbPath := filepath.Join(dir, "db.bolt")
	return New(dbPath)
}

// New opens (or creates) the Bolt file and bucket.
// dbPath should be a .bolt file on disk.
func New(dbPath string) (*Deduplicator, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}
	d := &Deduplicator{db: db, bucket: []byte("proxies")}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(d.bucket)
		return err
	}); err != nil {
		db.Close()
		return nil, err
	}
	return d, nil
}

// ShouldProcess returns true if the proxy has never been marked,
// or if now – lastMarked ≥ age.
// On any internal error it returns true as a fail‐safe.
func (d *Deduplicator) ShouldProcess(p domain.ProvidedProxy, age time.Duration) bool {
	var lastNanos uint64

	err := d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		v := b.Get(p.Key())
		// if missing or corrupted, treat as never seen
		if len(v) != 8 {
			lastNanos = 0
		} else {
			lastNanos = binary.BigEndian.Uint64(v)
		}
		return nil
	})
	if err != nil {
		// fail‐safe: on any read error, allow processing
		return true
	}

	// never seen before
	if lastNanos == 0 {
		return true
	}

	lastTime := time.Unix(0, int64(lastNanos))
	return time.Since(lastTime) >= age
}

// MarkProcessed records the current timestamp for this proxy.
func (d *Deduplicator) MarkProcessed(p domain.ProvidedProxy) error {
	now := uint64(time.Now().UnixNano())
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, now)

	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(d.bucket)
		return b.Put(p.Key(), buf)
	})
}

// Close closes the underlying Bolt DB.
func (d *Deduplicator) Close() error {
	return d.db.Close()
}
