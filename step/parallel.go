package step

import (
	"errors"
	"runtime"
	"sync"
)

// parallelChunk is the smallest DATA-section chunk worth a goroutine. Below it
// the fixed cost of a parser and its slab outweighs the work.
const parallelChunk = 1 << 20

// errSplit reports that a chunk boundary did not fall between two records, so
// the chunks' results cannot be joined.
var errSplit = errors.New("step: chunk boundary is not a record boundary")

// parseParallel parses src with the DATA section split across GOMAXPROCS
// parsers. It reports ok=false, and the caller parses serially, whenever the
// file is too small to gain, has no DATA section, fails to parse anywhere, or
// was split somewhere other than between two records. The serial parse is the
// reference: parseParallel returns a result only when it is the same one, and
// leaves every error, with its exact offset, to the serial path.
func parseParallel(src []byte) (*File, bool) {
	return parseParallelN(src, runtime.GOMAXPROCS(0))
}

func parseParallelN(src []byte, workers int) (*File, bool) {
	if workers < 2 || len(src) < 2*parallelChunk {
		return nil, false
	}
	f := &File{}
	head := newParser(src, f, 0, 0)
	head.stopAtData = true
	if err := head.parseDocument(); err != nil || !head.atData {
		return nil, false
	}
	bounds := splitRecords(src, head.s.pos, workers)
	if len(bounds) < 3 {
		return nil, false
	}
	chunks := len(bounds) - 1
	ps := make([]*parser, chunks)
	errs := make([]error, chunks)
	var wg sync.WaitGroup
	for i := range chunks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := newParser(src, f, uint16(i+1), bounds[i+1]-bounds[i])
			p.s.pos = bounds[i]
			ps[i] = p
			if i < chunks-1 {
				errs[i] = p.parseChunk(bounds[i+1])
				return
			}
			// The last chunk runs to ENDSEC and on through whatever follows
			// it, as the serial parser would.
			if errs[i] = p.parseRecords(); errs[i] == nil {
				errs[i] = p.parseDocument()
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, false
		}
	}
	finish(f, append([]*parser{head}, ps...))
	return f, true
}

// parseChunk parses whole records until the scanner reaches end, the start of
// the next chunk. It fails with errSplit unless the last record ends exactly
// there: a boundary that fell inside a string or a comment surfaces as a
// record running past end, or as a chunk that does not begin with a record.
func (p *parser) parseChunk(end int) error {
	for {
		p.s.skipTrivia()
		switch {
		case p.s.pos == end:
			return nil
		case p.s.pos > end:
			return errSplit
		}
		tok := p.s.Next()
		if tok.Kind != TokRef {
			return errSplit
		}
		if err := p.parseInstance(tok); err != nil {
			return err
		}
	}
}

// splitRecords returns chunk boundaries for the DATA section starting at start:
// start itself, then up to workers-1 offsets, each the '#' of the first record
// that follows a ';' at or after an even share of the remaining bytes, then
// len(src). The boundaries are guesses that parseChunk verifies.
func splitRecords(src []byte, start, workers int) []int {
	n := min(workers, (len(src)-start)/parallelChunk)
	bounds := []int{start}
	for i := 1; i < n; i++ {
		b := nextRecord(src, start+(len(src)-start)*i/n)
		if b < 0 {
			break
		}
		if b > bounds[len(bounds)-1] {
			bounds = append(bounds, b)
		}
	}
	return append(bounds, len(src))
}

// nextRecord returns the offset of the '#' that begins the first record after a
// ';' at or past from, or -1.
func nextRecord(src []byte, from int) int {
	for i := from; i < len(src); i++ {
		if src[i] != ';' {
			continue
		}
		j := i + 1
		for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r' || src[j] == '\n') {
			j++
		}
		if j < len(src) && src[j] == '#' {
			return j
		}
	}
	return -1
}
