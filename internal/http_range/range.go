package http_range

import (
	"errors"
	"strconv"
	"strings"
)

type Range struct {
	Start int64
	End   int64
}

var (
	ErrNoOverlap = errors.New("invalid range: failed to overlap")

	ErrInvalid = errors.New("invalid range")
)

func Parse(header string, size int64) ([]*Range, error) {
	index := strings.Index(header, "=")

	if index == -1 {
		return nil, ErrInvalid
	}

	arr := strings.Split(header[index+1:], ",")
	ranges := make([]*Range, 0, len(arr))

	for _, value := range arr {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		r := strings.Split(value, "-")
		// Range must be in format "start-end" (exactly 2 parts)
		if len(r) != 2 {
			continue
		}

		startStr := strings.TrimSpace(r[0])
		endStr := strings.TrimSpace(r[1])

		var start, end int64
		var startErr, endErr error

		// Parse start if provided
		if startStr != "" {
			start, startErr = strconv.ParseInt(startStr, 10, 64)
		} else {
			startErr = errors.New("empty start")
		}

		// Parse end if provided
		if endStr != "" {
			end, endErr = strconv.ParseInt(endStr, 10, 64)
		} else {
			endErr = errors.New("empty end")
		}

		// Both empty is invalid
		if startErr != nil && endErr != nil {
			continue
		}

		// Handle suffix-byte-range: "-nnn" means last nnn bytes
		if startErr != nil {
			if end > size {
				end = size
			}
			if end <= 0 {
				continue
			}
			start = size - end
			end = size - 1
			if start < 0 {
				start = 0
			}
		} else if endErr != nil {
			// Open-ended range: "nnn-" means from nnn to end
			end = size - 1
		}

		// Clamp end to file size
		if end >= size {
			end = size - 1
		}

		// Validate range
		if start < 0 || start > end || end < 0 {
			continue
		}

		ranges = append(ranges, &Range{
			Start: start,
			End:   end,
		})
	}

	if len(ranges) == 0 {
		return nil, ErrNoOverlap
	}

	return ranges, nil
}
