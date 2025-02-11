// +build windows

package headerfs

import (
	"fmt"
	"os"
)

// singleTruncate truncates a single header from the end of the header file. This can be used in the case of a re-org to
// remove the last header from the end of the main chain.
//
// TODO(roasbeef): define this and the two methods above on a headerFile struct?
func (h *headerStore) singleTruncate() (e error) {
	// In order to truncate the file, we'll need to grab the absolute size of the file as it stands currently.
	fileInfo, e := h.file.Stat()
	if e != nil {
		return e
	}
	fileSize := fileInfo.Size()
	// Next, we'll determine the number of bytes we need to truncate from the end of the file.
	var truncateLength int64
	switch h.indexType {
	case Block:
		truncateLength = 80
	case RegularFilter:
		truncateLength = 32
	default:
		return fmt.Errorf("unknown index type: %v", h.indexType)
	}
	// Finally, we'll use both of these values to calculate the new size of the file.
	newSize := fileSize - truncateLength
	// On Windows, a file can't be truncated while open, even if using a file handle to truncate it. This means we have
	// to close, truncate, and reopen it.
	fileName := h.file.Name()
	if e = h.file.Close(); E.Chk(e) {
		return e
	}
	if e = os.Truncate(fileName, newSize); E.Chk(e) {
		return e
	}
	fileFlags := os.O_RDWR | os.O_APPEND | os.O_CREATE
	h.file, e = os.OpenFile(fileName, fileFlags, 0644)
	return e
}
