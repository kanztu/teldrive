package reader

import (
	"context"
	"fmt"
	"io"

	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/pkg/models"
	"github.com/tgdrive/teldrive/pkg/types"
)

type Range struct {
	Start, End int64
	PartNo     int64
}

type Reader struct {
	ctx         context.Context
	file        *models.File
	parts       []types.Part
	ranges      []Range
	pos         int
	reader      io.ReadCloser
	remaining   int64
	config      *config.TGConfig
	client      *tg.Client
	concurrency int
	cache       cache.Cacher
}

func calculatePartByteRanges(start, end, partSize int64) []Range {
	ranges := make([]Range, 0)
	startPart := start / partSize
	endPart := end / partSize

	for part := startPart; part <= endPart; part++ {
		partStart := max(start-part*partSize, 0)
		partEnd := min(partSize-1, end-part*partSize)
		ranges = append(ranges, Range{
			Start:  partStart,
			End:    partEnd,
			PartNo: part,
		})
	}
	return ranges
}

func NewReader(ctx context.Context,
	client *tg.Client,
	cache cache.Cacher,
	file *models.File,
	parts []types.Part,
	start,
	end int64,
	config *config.TGConfig,
) (io.ReadCloser, error) {

	size := parts[0].Size
	if *file.Encrypted {
		size = parts[0].DecryptedSize
	}
	r := &Reader{
		ctx:       ctx,
		parts:     parts,
		file:      file,
		remaining: end - start + 1,
		ranges:    calculatePartByteRanges(start, end, size),
		config:    config,
		client:    client,
		cache:     cache,
	}

	if err := r.initializeReader(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)

	if err == io.EOF && r.remaining > 0 {
		if err := r.moveToNextPart(); err != nil {
			return n, err
		}
		err = nil
	}

	return n, err
}

func (r *Reader) Close() error {
	if r.reader != nil {
		err := r.reader.Close()
		r.reader = nil
		return err
	}
	return nil
}

func (r *Reader) initializeReader() error {
	reader, err := r.getPartReader()
	if err != nil {
		return err
	}
	r.reader = reader
	return nil
}

func (r *Reader) moveToNextPart() error {
	r.reader.Close()
	r.pos++
	if r.pos < len(r.ranges) {
		return r.initializeReader()
	}
	return io.EOF
}

func (r *Reader) getPartReader() (io.ReadCloser, error) {
	// Bounds check for ranges position
	if r.pos >= len(r.ranges) {
		return nil, fmt.Errorf("position %d out of range (max %d)", r.pos, len(r.ranges)-1)
	}

	currentRange := r.ranges[r.pos]

	// Bounds check for parts array
	if currentRange.PartNo < 0 || currentRange.PartNo >= int64(len(r.parts)) {
		return nil, fmt.Errorf("part number %d out of range (available parts: %d)", currentRange.PartNo, len(r.parts))
	}

	partId := r.parts[currentRange.PartNo].ID

	chunkSrc := &chunkSource{
		channelId:   *r.file.ChannelId,
		partId:      partId,
		client:      r.client,
		concurrency: r.concurrency,
		cache:       r.cache,
		key:         cache.Key("files", "location", r.file.ID, partId),
	}

	var (
		reader io.ReadCloser
		err    error
	)

	reader, err = newTGMultiReader(r.ctx, currentRange.Start, currentRange.End, r.config, chunkSrc)

	if *r.file.Encrypted {
		// Additional bounds check for encrypted files
		if r.pos >= len(r.ranges) || r.ranges[r.pos].PartNo >= int64(len(r.parts)) {
			return nil, fmt.Errorf("invalid range or part index during encryption: pos=%d, partNo=%d", r.pos, r.ranges[r.pos].PartNo)
		}

		salt := r.parts[r.ranges[r.pos].PartNo].Salt
		cipher, _ := crypt.NewCipher(r.config.Uploads.EncryptionKey, salt)
		reader, err = cipher.DecryptDataSeek(r.ctx,
			func(ctx context.Context,
				underlyingOffset,
				underlyingLimit int64) (io.ReadCloser, error) {
				var end int64

				if underlyingLimit >= 0 {
					// Additional bounds check before accessing parts
					if r.pos >= len(r.ranges) || r.ranges[r.pos].PartNo >= int64(len(r.parts)) {
						return nil, fmt.Errorf("invalid part index in decryption callback")
					}
					end = min(r.parts[r.ranges[r.pos].PartNo].Size-1, underlyingOffset+underlyingLimit-1)
				}

				return newTGMultiReader(r.ctx, underlyingOffset, end, r.config, chunkSrc)

			}, currentRange.Start, currentRange.End-currentRange.Start+1)
	}

	return reader, err

}
