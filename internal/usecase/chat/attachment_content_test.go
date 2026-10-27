package chat

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	domain "github.com/janickiy/meet-space/internal/domain/chat"
)

// attachmentHeader builds signature-only fixtures; these are not complete documents.
// prefix supplies the container marker and size is the required header length.
func attachmentHeader(prefix string, size int) []byte {
	data := make([]byte, size)
	copy(data, prefix)
	return data
}

// TestDocumentAndArchiveContent validates new formats through the upload validator.
func TestDocumentAndArchiveContent(t *testing.T) {
	var zipped bytes.Buffer
	if err := zip.NewWriter(&zipped).Close(); err != nil {
		t.Fatal(err)
	}
	var tarred bytes.Buffer
	if err := tar.NewWriter(&tarred).Close(); err != nil {
		t.Fatal(err)
	}
	ole := attachmentHeader("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1", 512)
	one := attachmentHeader("\xe4\x52\x5c\x7b\x8c\xd8\xa7\x4d\xae\xb1\x53\x78\xd0\x29\x96\xd3", 1024)
	copy(one[48:64], []byte("\x3f\xdd\x9a\x10\x1b\x91\xf5\x49\xa5\xd0\x17\x91\xed\xc8\xae\xd8"))
	mobi := make([]byte, 100)
	copy(mobi[60:68], "BOOKMOBI")
	binary.BigEndian.PutUint16(mobi[76:78], 1)
	binary.BigEndian.PutUint32(mobi[78:82], 86)
	bz3 := attachmentHeader("BZ3v1", 9)
	binary.LittleEndian.PutUint32(bz3[5:9], 1<<20)
	ace := make([]byte, 20)
	copy(ace[7:14], "**ACE**")
	arc := attachmentHeader("\x1a\x08file.txt\x00", 29)
	zpaq := attachmentHeader("zPQ\x02\x01\x07\x00", 14)
	djvu := attachmentHeader("AT&TFORM", 16)
	copy(djvu[12:16], "DJVU")
	for _, fixture := range []struct {
		extensions string
		data       []byte
	}{
		{"zip zipx docx xlsx pptx odt ods odp pages numbers key epub xps oxps odm oth otm odb sldx sldm vsdx vstx vssx vsdm vstm vssm", zipped.Bytes()},
		{"doc dot xls xlt ppt pot pps vsd vst vss vsw", ole},
		{"one onetoc2", one},
		{"mobi prc azw azw3", mobi},
		{"djvu djv", djvu},
		{"fb2", []byte(`<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0"><body/></FictionBook>`)},
		{"xml", []byte(`<?xml version="1.0"?><document><title>Meeting</title></document>`)},
		{"json", []byte(`{"meeting":"MeetSpace","files":[1,2]}`)},
		{"yaml yml", []byte("meeting: MeetSpace\nfiles:\n  - report.pdf\n")},
		{"tex ltx latex", []byte("\\documentclass{article}\n\\begin{document}Meeting\\end{document}")},
		{"txt md markdown rst adoc asciidoc", []byte("Meeting notes\nDecisions and next steps.\n")},
		{"csv tsv", []byte("name,value\nmeeting,1\n")},
		{"ps eps", []byte("%!PS-Adobe-3.0\n%%Title: Meeting\n")},
		{"pdf", []byte("%PDF-1.7\nsynthetic signature fixture\n")},
		{"rtf", []byte("{\\rtf1 Meeting notes}")},
		{"bz bzip", []byte("BZ01")},
		{"bz2 bzip2 tbz tbz2", []byte("BZh9")},
		{"bz3 bzip3", bz3},
		{"lzo lzop tzo", attachmentHeader("\x89LZO\x00\r\n\x1a\n", 25)},
		{"ace", ace},
		{"arc", arc},
		{"zpaq", zpaq},
		{"gz gzip tgz", attachmentHeader("\x1f\x8b\x08", 10)},
		{"tar", tarred.Bytes()},
		{"7z", attachmentHeader("7z\xbc\xaf\x27\x1c", 32)},
		{"rar", []byte("Rar!\x1a\x07\x01\x00")},
		{"xz txz", attachmentHeader("\xfd7zXZ\x00", 12)},
		{"zst zstd tzst", []byte("\x28\xb5\x2f\xfd")},
		{"lz4", []byte("\x04\x22\x4d\x18")},
		{"lz lzip", attachmentHeader("LZIP\x01", 6)},
		{"lzma", attachmentHeader("\x5d\x00\x00\x80\x00", 13)},
		{"z taz", []byte("\x1f\x9d\x90")},
		{"cab", attachmentHeader("MSCF", 36)},
		{"cpio", attachmentHeader("070701", 26)},
		{"ar", []byte("!<arch>\n")},
		{"arj", attachmentHeader("\x60\xea", 6)},
		{"lha lzh", attachmentHeader("\x00\x00-lh5-", 22)},
	} {
		for extension := range strings.FieldsSeq(fixture.extensions) {
			t.Run(extension, func(t *testing.T) {
				filename := "document." + extension
				mediaType, ok := domain.AttachmentMIMEType(filename)
				if !ok {
					t.Fatalf("extension %s is missing from policy", extension)
				}
				attachment := domain.Attachment{Filename: filename, MimeType: mediaType, Size: int64(len(fixture.data))}
				if err := ValidateContent(fixture.data, attachment); err != nil {
					t.Fatalf("valid signature rejected: %v", err)
				}
				malicious := []byte("MZ\x00\x00not a document")
				attachment.Size = int64(len(malicious))
				if err := ValidateContent(malicious, attachment); err == nil {
					t.Fatal("accepted an executable disguised as a document")
				}
			})
		}
	}
}

// TestStructuredAttachmentsRejectUnsafeInput protects bounded, inert processing.
func TestStructuredAttachmentsRejectUnsafeInput(t *testing.T) {
	for _, data := range []string{
		`<!DOCTYPE root [<!ENTITY x SYSTEM "file:///etc/passwd">]><root>&x;</root>`,
		`<root/><other/>`, `<root>`, `<html/>`, `<svg/>`, `<root><script/></root>`,
		strings.Repeat("<root>", 129) + strings.Repeat("</root>", 129),
		"<root>\x00</root>", "<root>\xff</root>",
		"<root>" + strings.Repeat("<child/>", 125_000) + "</root>",
	} {
		if xmlDocumentContent([]byte(data), false) {
			t.Errorf("accepted unsafe XML of %d bytes", len(data))
		}
	}
	if xmlDocumentContent([]byte("<FictionBook/>"), true) {
		t.Error("accepted FictionBook without its namespace")
	}
	for _, data := range []string{"<html>document</html>", "#!/bin/sh\necho unsafe", "MZ\x00\x00", "\xfftext"} {
		if plainDocumentContent([]byte(data)) {
			t.Errorf("accepted active or binary text %q", data)
		}
	}
	// Alias-heavy YAML stays raw text; the API never expands it into objects.
	if !documentOrArchiveContent([]byte("a: &a [1, 2]\nb: [*a, *a]\n"), "application/yaml") {
		t.Error("rejected an inert YAML download")
	}
	attachment := domain.Attachment{Filename: "data.json", MimeType: "application/json", Size: 2}
	if ValidateContent([]byte("{}"), attachment) != nil {
		t.Error("rejected JSON")
	}
	attachment.Size++
	if ValidateContent([]byte("{}"), attachment) == nil {
		t.Error("accepted an incorrect declared length")
	}
}

// FuzzAttachmentContent checks that arbitrary upload bytes cannot panic validation.
func FuzzAttachmentContent(f *testing.F) {
	for _, seed := range []struct{ data, filename string }{{"<root/>", "document.xml"}, {"BZh9", "archive.bz2"}, {"{}", "document.json"}, {"MZ\x00", "document.one"}} {
		f.Add([]byte(seed.data), seed.filename)
	}
	f.Fuzz(func(t *testing.T, data []byte, filename string) {
		if int64(len(data)) > domain.MaxAttachmentBytes {
			return
		}
		mediaType, _ := domain.AttachmentMIMEType(filename)
		_ = ValidateContent(data, domain.Attachment{Filename: filename, MimeType: mediaType, Size: int64(len(data))})
	})
}
