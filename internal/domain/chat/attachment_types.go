package chat

import (
	"mime"
	"path/filepath"
	"strings"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

type attachmentType struct {
	mime    string
	aliases []string
}

// zipDocument declares a document container and MIME aliases reported by browsers.
// The content validator still checks the uploaded container independently.
func zipDocument(mediaType string) attachmentType {
	return attachmentType{mime: mediaType, aliases: []string{"application/zip", "application/x-zip", "application/x-zip-compressed"}}
}

var attachmentTypes = map[string]attachmentType{
	".jpg":      {mime: "image/jpeg"},
	".jpeg":     {mime: "image/jpeg"},
	".png":      {mime: "image/png"},
	".webp":     {mime: "image/webp"},
	".pdf":      {mime: "application/pdf", aliases: []string{"application/x-pdf"}},
	".txt":      {mime: "text/plain"},
	".csv":      {mime: "text/csv", aliases: []string{"text/plain", "application/csv", "application/vnd.ms-excel"}},
	".tsv":      {mime: "text/tab-separated-values", aliases: []string{"text/plain"}},
	".md":       {mime: "text/markdown", aliases: []string{"text/plain", "text/x-markdown"}},
	".markdown": {mime: "text/markdown", aliases: []string{"text/plain", "text/x-markdown"}},
	".rtf":      {mime: "application/rtf", aliases: []string{"text/rtf"}},
	".doc":      {mime: "application/msword"},
	".dot":      {mime: "application/msword"},
	".xls":      {mime: "application/vnd.ms-excel"},
	".xlt":      {mime: "application/vnd.ms-excel"},
	".ppt":      {mime: "application/vnd.ms-powerpoint"},
	".pot":      {mime: "application/vnd.ms-powerpoint"},
	".pps":      {mime: "application/vnd.ms-powerpoint"},
	".docx":     zipDocument("application/vnd.openxmlformats-officedocument.wordprocessingml.document"),
	".dotx":     zipDocument("application/vnd.openxmlformats-officedocument.wordprocessingml.template"),
	".xlsx":     zipDocument("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"),
	".xltx":     zipDocument("application/vnd.openxmlformats-officedocument.spreadsheetml.template"),
	".pptx":     zipDocument("application/vnd.openxmlformats-officedocument.presentationml.presentation"),
	".potx":     zipDocument("application/vnd.openxmlformats-officedocument.presentationml.template"),
	".ppsx":     zipDocument("application/vnd.openxmlformats-officedocument.presentationml.slideshow"),
	".docm":     zipDocument("application/vnd.ms-word.document.macroenabled.12"),
	".dotm":     zipDocument("application/vnd.ms-word.template.macroenabled.12"),
	".xlsm":     zipDocument("application/vnd.ms-excel.sheet.macroenabled.12"),
	".xlsb":     zipDocument("application/vnd.ms-excel.sheet.binary.macroenabled.12"),
	".xltm":     zipDocument("application/vnd.ms-excel.template.macroenabled.12"),
	".pptm":     zipDocument("application/vnd.ms-powerpoint.presentation.macroenabled.12"),
	".potm":     zipDocument("application/vnd.ms-powerpoint.template.macroenabled.12"),
	".ppsm":     zipDocument("application/vnd.ms-powerpoint.slideshow.macroenabled.12"),
	".odt":      zipDocument("application/vnd.oasis.opendocument.text"),
	".ods":      zipDocument("application/vnd.oasis.opendocument.spreadsheet"),
	".odp":      zipDocument("application/vnd.oasis.opendocument.presentation"),
	".odg":      zipDocument("application/vnd.oasis.opendocument.graphics"),
	".odf":      zipDocument("application/vnd.oasis.opendocument.formula"),
	".ott":      zipDocument("application/vnd.oasis.opendocument.text-template"),
	".ots":      zipDocument("application/vnd.oasis.opendocument.spreadsheet-template"),
	".otp":      zipDocument("application/vnd.oasis.opendocument.presentation-template"),
	".otg":      zipDocument("application/vnd.oasis.opendocument.graphics-template"),
	".pages":    zipDocument("application/vnd.apple.pages"),
	".numbers":  zipDocument("application/vnd.apple.numbers"),
	".key":      zipDocument("application/vnd.apple.keynote"),
	".epub":     zipDocument("application/epub+zip"),
	".zip":      {mime: "application/zip", aliases: []string{"application/x-zip", "application/x-zip-compressed"}},
	".zipx":     {mime: "application/zip", aliases: []string{"application/x-zip", "application/x-zip-compressed"}},
	".7z":       {mime: "application/x-7z-compressed"},
	".rar":      {mime: "application/vnd.rar", aliases: []string{"application/x-rar", "application/x-rar-compressed"}},
	".tar":      {mime: "application/x-tar", aliases: []string{"application/x-gtar"}},
	".gz":       {mime: "application/gzip", aliases: []string{"application/x-gzip", "application/gzip-compressed"}},
	".gzip":     {mime: "application/gzip", aliases: []string{"application/x-gzip"}},
	".tgz":      {mime: "application/gzip", aliases: []string{"application/x-gzip", "application/x-compressed-tar"}},
	".bz2":      {mime: "application/x-bzip2", aliases: []string{"application/bzip2", "application/x-bz2"}},
	".bzip2":    {mime: "application/x-bzip2", aliases: []string{"application/bzip2"}},
	".tbz":      {mime: "application/x-bzip2", aliases: []string{"application/x-bzip-compressed-tar"}},
	".tbz2":     {mime: "application/x-bzip2", aliases: []string{"application/x-bzip-compressed-tar"}},
	".xz":       {mime: "application/x-xz"},
	".txz":      {mime: "application/x-xz", aliases: []string{"application/x-xz-compressed-tar"}},
	".zst":      {mime: "application/zstd", aliases: []string{"application/x-zstd"}},
	".zstd":     {mime: "application/zstd", aliases: []string{"application/x-zstd"}},
	".tzst":     {mime: "application/zstd", aliases: []string{"application/x-zstd", "application/x-zstd-compressed-tar"}},
	".lz4":      {mime: "application/x-lz4", aliases: []string{"application/lz4"}},
	".lz":       {mime: "application/x-lzip", aliases: []string{"application/lzip"}},
	".lzip":     {mime: "application/x-lzip", aliases: []string{"application/lzip"}},
	".lzma":     {mime: "application/x-lzma"},
	".z":        {mime: "application/x-compress"},
	".taz":      {mime: "application/x-compress", aliases: []string{"application/x-tarz"}},
	".cab":      {mime: "application/vnd.ms-cab-compressed", aliases: []string{"application/x-cab", "application/x-cab-compressed"}},
	".cpio":     {mime: "application/x-cpio"},
	".ar":       {mime: "application/x-archive", aliases: []string{"application/x-ar"}},
	".arj":      {mime: "application/x-arj", aliases: []string{"application/arj"}},
	".lha":      {mime: "application/x-lzh-compressed", aliases: []string{"application/x-lha", "application/x-lha-compressed"}},
	".lzh":      {mime: "application/x-lzh-compressed", aliases: []string{"application/x-lha", "application/x-lha-compressed"}},
	".bz":       {mime: "application/x-bzip", aliases: []string{"application/x-bzip2"}},
	".bzip":     {mime: "application/x-bzip", aliases: []string{"application/x-bzip2"}},
	".bz3":      {mime: "application/x-bzip3"},
	".bzip3":    {mime: "application/x-bzip3"},
	".lzo":      {mime: "application/x-lzop", aliases: []string{"application/x-lzo"}},
	".lzop":     {mime: "application/x-lzop", aliases: []string{"application/x-lzo"}},
	".tzo":      {mime: "application/x-lzop", aliases: []string{"application/x-lzop-compressed-tar"}},
	".ace":      {mime: "application/x-ace-compressed"},
	".arc":      {mime: "application/x-arc"},
	".zpaq":     {mime: "application/x-zpaq"},
	".djvu":     {mime: "image/vnd.djvu", aliases: []string{"image/x-djvu"}},
	".djv":      {mime: "image/vnd.djvu", aliases: []string{"image/x-djvu"}},
	".fb2":      {mime: "application/x-fictionbook+xml", aliases: []string{"text/xml", "application/xml"}},
	".mobi":     {mime: "application/x-mobipocket-ebook"},
	".prc":      {mime: "application/x-mobipocket-ebook"},
	".azw":      {mime: "application/vnd.amazon.ebook", aliases: []string{"application/x-mobipocket-ebook"}},
	".azw3":     {mime: "application/vnd.amazon.ebook", aliases: []string{"application/x-mobipocket-ebook"}},
	".xps":      {mime: "application/vnd.ms-xpsdocument", aliases: []string{"application/zip"}},
	".oxps":     {mime: "application/oxps", aliases: []string{"application/zip"}},
	".ps":       {mime: "application/postscript"},
	".eps":      {mime: "application/postscript"},
	".xml":      {mime: "application/xml", aliases: []string{"text/xml", "text/plain"}},
	".json":     {mime: "application/json", aliases: []string{"text/json", "text/plain"}},
	".yaml":     {mime: "application/yaml", aliases: []string{"application/x-yaml", "text/yaml", "text/x-yaml", "text/plain"}},
	".yml":      {mime: "application/yaml", aliases: []string{"application/x-yaml", "text/yaml", "text/x-yaml", "text/plain"}},
	".odm":      {mime: "application/vnd.oasis.opendocument.text-master", aliases: []string{"application/zip"}},
	".oth":      {mime: "application/vnd.oasis.opendocument.text-web", aliases: []string{"application/zip"}},
	".otm":      {mime: "application/vnd.oasis.opendocument.text-master-template", aliases: []string{"application/zip"}},
	".odb":      {mime: "application/vnd.oasis.opendocument.database", aliases: []string{"application/zip"}},
	".sldx":     {mime: "application/vnd.openxmlformats-officedocument.presentationml.slide", aliases: []string{"application/zip"}},
	".sldm":     {mime: "application/vnd.ms-powerpoint.slide.macroenabled.12", aliases: []string{"application/zip"}},
	".vsd":      {mime: "application/vnd.visio"},
	".vst":      {mime: "application/vnd.visio"},
	".vss":      {mime: "application/vnd.visio"},
	".vsw":      {mime: "application/vnd.visio"},
	".vsdx":     {mime: "application/vnd.ms-visio.drawing", aliases: []string{"application/vnd.visio", "application/zip"}},
	".vstx":     {mime: "application/vnd.ms-visio.template", aliases: []string{"application/vnd.visio", "application/zip"}},
	".vssx":     {mime: "application/vnd.ms-visio.stencil", aliases: []string{"application/vnd.visio", "application/zip"}},
	".vsdm":     {mime: "application/vnd.ms-visio.drawing.macroenabled.12", aliases: []string{"application/vnd.visio", "application/zip"}},
	".vstm":     {mime: "application/vnd.ms-visio.template.macroenabled.12", aliases: []string{"application/vnd.visio", "application/zip"}},
	".vssm":     {mime: "application/vnd.ms-visio.stencil.macroenabled.12", aliases: []string{"application/vnd.visio", "application/zip"}},
	".one":      {mime: "application/onenote", aliases: []string{"application/msonenote", "application/x-onenote"}},
	".onetoc2":  {mime: "application/onenote", aliases: []string{"application/msonenote", "application/x-onenote"}},
	".tex":      {mime: "application/x-tex", aliases: []string{"text/x-tex", "text/plain"}},
	".ltx":      {mime: "application/x-tex", aliases: []string{"text/x-tex", "text/plain"}},
	".latex":    {mime: "application/x-tex", aliases: []string{"application/x-latex", "text/x-tex", "text/plain"}},
	".rst":      {mime: "text/plain"},
	".adoc":     {mime: "text/plain"},
	".asciidoc": {mime: "text/plain"},
}

// AttachmentMIMEType returns the canonical MIME type for a supported filename.
// The final extension also covers compound archive names such as backup.tar.gz.
func AttachmentMIMEType(filename string) (string, bool) {
	format, ok := attachmentTypes[strings.ToLower(filepath.Ext(filename))]
	return format.mime, ok
}

// normalizeAttachmentMIME checks a browser MIME hint and returns the canonical type.
// Empty or generic hints remain subject to independent upload-content validation.
func normalizeAttachmentMIME(filename, hint string) (string, error) {
	format, ok := attachmentTypes[strings.ToLower(filepath.Ext(filename))]
	if !ok {
		return "", apperrors.New(apperrors.ErrInvalidInput, "unsupported attachment extension")
	}
	if hint == "" {
		return format.mime, nil
	}
	mediaType, _, err := mime.ParseMediaType(hint)
	if err == nil && (mediaType == format.mime || mediaType == "application/octet-stream") {
		return format.mime, nil
	}
	if err == nil {
		// Desktop browsers can report the compound TAR format instead of the outer compressor.
		compound := map[string]string{
			"application/x-compressed-tar":      "application/gzip",
			"application/x-bzip-compressed-tar": "application/x-bzip2",
			"application/x-xz-compressed-tar":   "application/x-xz",
			"application/x-zstd-compressed-tar": "application/zstd",
			"application/x-lzma-compressed-tar": "application/x-lzma",
			"application/x-lzip-compressed-tar": "application/x-lzip",
		}
		if compound[mediaType] == format.mime {
			return format.mime, nil
		}
		for _, alias := range format.aliases {
			if mediaType == alias {
				return format.mime, nil
			}
		}
	}
	return "", apperrors.New(apperrors.ErrInvalidInput, "filename and MIME type disagree")
}
