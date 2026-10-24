package chat

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"io"
	"strings"
)

// documentOrArchiveContent checks container signatures without unpacking their contents.
// mediaType must be a canonical type allowed by the attachment filename policy.
func documentOrArchiveContent(data []byte, mediaType string) bool {
	zip := (len(data) >= 30 && bytes.HasPrefix(data, []byte("PK\x03\x04"))) || (len(data) >= 22 && bytes.HasPrefix(data, []byte("PK\x05\x06"))) || (len(data) >= 16 && bytes.HasPrefix(data, []byte("PK\x07\x08")))
	ole := len(data) >= 512 && bytes.HasPrefix(data, []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"))
	if strings.HasPrefix(mediaType, "application/vnd.openxmlformats-officedocument.") || strings.Contains(mediaType, ".macroenabled.12") {
		// Password-protected Office documents can use an OLE encryption container.
		return zip || ole
	}
	if strings.HasPrefix(mediaType, "application/vnd.oasis.opendocument.") {
		return zip
	}
	switch mediaType {
	case "application/zip", "application/epub+zip", "application/vnd.apple.pages", "application/vnd.apple.numbers", "application/vnd.apple.keynote":
		return zip
	case "application/msword":
		return ole || bytes.HasPrefix(data, []byte("{\\rtf"))
	case "application/vnd.ms-excel", "application/vnd.ms-powerpoint":
		return ole
	case "application/rtf":
		return bytes.HasPrefix(data, []byte("{\\rtf"))
	case "application/gzip":
		return len(data) >= 10 && bytes.HasPrefix(data, []byte("\x1f\x8b\x08")) && data[3]&0xe0 == 0
	case "application/x-bzip2":
		return len(data) >= 4 && bytes.HasPrefix(data, []byte("BZh")) && data[3] >= '1' && data[3] <= '9'
	case "application/x-7z-compressed":
		return len(data) >= 32 && bytes.HasPrefix(data, []byte("7z\xbc\xaf\x27\x1c"))
	case "application/vnd.rar":
		return bytes.HasPrefix(data, []byte("Rar!\x1a\x07\x00")) || bytes.HasPrefix(data, []byte("Rar!\x1a\x07\x01\x00"))
	case "application/x-tar":
		if len(data) < 512 {
			return false
		}
		_, err := tar.NewReader(bytes.NewReader(data)).Next()
		return err == nil || (err == io.EOF && len(data) >= 1024 && bytes.Equal(data[:1024], make([]byte, 1024)))
	case "application/x-xz":
		return len(data) >= 12 && bytes.HasPrefix(data, []byte("\xfd7zXZ\x00"))
	case "application/zstd":
		return len(data) >= 4 && (binary.LittleEndian.Uint32(data[:4]) == 0xfd2fb528 || binary.LittleEndian.Uint32(data[:4])&0xfffffff0 == 0x184d2a50)
	case "application/x-lz4":
		return len(data) >= 4 && (binary.LittleEndian.Uint32(data[:4]) == 0x184d2204 || binary.LittleEndian.Uint32(data[:4]) == 0x184c2102 || binary.LittleEndian.Uint32(data[:4])&0xfffffff0 == 0x184d2a50)
	case "application/x-lzip":
		return len(data) >= 6 && bytes.HasPrefix(data, []byte("LZIP")) && data[4] <= 1
	case "application/x-lzma":
		if len(data) < 13 || data[0] >= 225 {
			return false
		}
		dictionary := binary.LittleEndian.Uint32(data[1:5])
		for shift := uint(12); shift <= 30; shift++ {
			if dictionary == 1<<shift || dictionary == 3<<(shift-1) {
				return true
			}
		}
		return false
	case "application/x-compress":
		return len(data) >= 3 && bytes.HasPrefix(data, []byte("\x1f\x9d")) && data[2]&0x60 == 0 && data[2]&0x1f >= 9 && data[2]&0x1f <= 16
	case "application/vnd.ms-cab-compressed":
		return len(data) >= 36 && bytes.HasPrefix(data, []byte("MSCF\x00\x00\x00\x00"))
	case "application/x-cpio":
		return len(data) >= 26 && (bytes.HasPrefix(data, []byte("070701")) || bytes.HasPrefix(data, []byte("070702")) || bytes.HasPrefix(data, []byte("070707")) || bytes.HasPrefix(data, []byte("\xc7\x71")) || bytes.HasPrefix(data, []byte("\x71\xc7")))
	case "application/x-archive":
		return bytes.HasPrefix(data, []byte("!<arch>\n")) || bytes.HasPrefix(data, []byte("!<thin>\n"))
	case "application/x-arj":
		return len(data) >= 6 && bytes.HasPrefix(data, []byte("\x60\xea"))
	case "application/x-lzh-compressed":
		if len(data) < 22 {
			return false
		}
		method := string(data[2:7])
		return (strings.HasPrefix(method, "-lh") && method[4] == '-' && ((method[3] >= '0' && method[3] <= '7') || method[3] == 'd')) || method == "-lz4-" || method == "-lz5-" || method == "-lzs-"
	default:
		return false
	}
}
