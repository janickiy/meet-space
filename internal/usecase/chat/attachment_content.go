package chat

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
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
	if strings.HasPrefix(mediaType, "application/vnd.ms-visio.") {
		return zip
	}
	switch mediaType {
	case "application/vnd.ms-xpsdocument", "application/oxps":
		return zip
	case "application/vnd.visio":
		return ole
	case "application/onenote":
		// MS-ONESTORE 2.3.1: 1024-byte revision-store header and both GUIDs.
		return len(data) >= 1024 && (bytes.HasPrefix(data, []byte("\xe4\x52\x5c\x7b\x8c\xd8\xa7\x4d\xae\xb1\x53\x78\xd0\x29\x96\xd3")) || bytes.HasPrefix(data, []byte("\xa1\x2f\xff\x43\xd9\xef\x76\x4c\x9e\xe2\x10\xea\x57\x22\x76\x5f"))) && bytes.Equal(data[48:64], []byte("\x3f\xdd\x9a\x10\x1b\x91\xf5\x49\xa5\xd0\x17\x91\xed\xc8\xae\xd8"))
	case "image/vnd.djvu":
		return len(data) >= 16 && bytes.HasPrefix(data, []byte("AT&TFORM")) && (bytes.Equal(data[12:16], []byte("DJVU")) || bytes.Equal(data[12:16], []byte("DJVM")) || bytes.Equal(data[12:16], []byte("DJVI")))
	case "application/x-mobipocket-ebook", "application/vnd.amazon.ebook":
		// Palm database header, a nonempty record table, and BOOKMOBI type/creator.
		return len(data) >= 86 && bytes.Equal(data[60:68], []byte("BOOKMOBI")) && binary.BigEndian.Uint16(data[76:78]) > 0 && int(binary.BigEndian.Uint16(data[76:78])) <= (len(data)-78)/8 && int(binary.BigEndian.Uint32(data[78:82])) >= 78+int(binary.BigEndian.Uint16(data[76:78]))*8 && int(binary.BigEndian.Uint32(data[78:82])) < len(data)
	case "application/postscript":
		return bytes.HasPrefix(data, []byte("%!PS")) || bytes.HasPrefix(data, []byte("\x04%!PS"))
	case "application/x-tex", "application/yaml":
		// These text formats are inert downloads. Never expand YAML aliases or
		// construct an object graph from an untrusted attachment on the API.
		return plainDocumentContent(data)
	case "application/xml", "application/x-fictionbook+xml":
		return xmlDocumentContent(data, mediaType == "application/x-fictionbook+xml")
	case "application/json":
		return utf8.Valid(data) && json.Valid(data)
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
	case "application/x-bzip":
		return len(data) >= 4 && (bytes.HasPrefix(data, []byte("BZ0")) || bytes.HasPrefix(data, []byte("BZh"))) && data[3] >= '1' && data[3] <= '9'
	case "application/x-bzip3":
		return len(data) >= 9 && bytes.HasPrefix(data, []byte("BZ3v1")) && binary.LittleEndian.Uint32(data[5:9]) >= 65*1024 && binary.LittleEndian.Uint32(data[5:9]) <= 511*1024*1024
	case "application/x-lzop":
		return len(data) >= 25 && bytes.HasPrefix(data, []byte("\x89LZO\x00\x0d\x0a\x1a\x0a"))
	case "application/x-ace-compressed":
		return len(data) >= 20 && bytes.Equal(data[7:14], []byte("**ACE**")) && data[4] == 0
	case "application/x-arc":
		return len(data) >= 29 && data[0] == 0x1a && data[1] >= 1 && data[1] <= 9 && bytes.IndexByte(data[2:15], 0) > 0
	case "application/x-zpaq":
		if bytes.HasPrefix(data, []byte("7kSt\xa01\x83\xd3\x8c\xb2\x28\xb0\xd3")) {
			data = data[13:]
		}
		return len(data) >= 14 && bytes.HasPrefix(data, []byte("zPQ")) && (data[3] == 1 || data[3] == 2) && data[4] == 1 && binary.LittleEndian.Uint16(data[5:7]) >= 7 && int(binary.LittleEndian.Uint16(data[5:7])) <= len(data)-7
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

// plainDocumentContent checks data for inert UTF-8 text without executing it.
// Binary, HTML and script-like content cannot pass by changing the extension.
func plainDocumentContent(data []byte) bool {
	return utf8.Valid(data) && !bytes.ContainsRune(data, 0) && !bytes.HasPrefix(bytes.TrimSpace(data), []byte("#!")) && strings.Split(http.DetectContentType(data), ";")[0] == "text/plain"
}

// xmlDocumentContent parses data without resolving external entities or DTDs.
// fictionBook additionally requires the FictionBook root and namespace.
// Active elements, multiple roots and excessive parsing work are rejected.
func xmlDocumentContent(data []byte, fictionBook bool) bool {
	if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return false
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for tokens := 0; tokens < 250_000; tokens++ {
		token, err := decoder.Token()
		if err == io.EOF {
			return roots == 1 && depth == 0
		}
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.Directive:
			return false
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots > 1 || fictionBook && (value.Name.Local != "FictionBook" || value.Name.Space != "http://www.gribuser.ru/xml/fictionbook/2.0") {
					return false
				}
			}
			depth++
			if depth > 128 {
				return false
			}
			switch strings.ToLower(value.Name.Local) {
			case "html", "script", "svg", "iframe", "object", "embed":
				return false
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(value)) > 0 {
				return false
			}
		}
	}
	return false
}
