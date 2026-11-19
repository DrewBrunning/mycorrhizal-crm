package services

import (
	"bytes"
	"errors"
)

// ErrImportNotText is returned by SniffImportText (and so by ParseCSV and the
// entity-CSV parser) when an upload is clearly binary rather than text: a
// photo, ZIP/XLSX, PDF or UTF-16 file renamed ".csv" (issue #2688). The HTTP
// layer maps it to a 400 with reason import_csv_not_text.
var ErrImportNotText = errors.New("file is not a text CSV file")

// importSniffWindow is how many leading bytes SniffImportText inspects.
const importSniffWindow = 8 * 1024

// binaryMagics are container/image signatures that never start a CSV. UTF-16
// byte-order marks are included: DecodeImportText only understands UTF-8 and
// Windows-1252, so a UTF-16 export (Excel's "Unicode Text") would otherwise
// import as NUL-riddled garbage.
var binaryMagics = [][]byte{
	[]byte("\x89PNG\r\n\x1a\n"),
	[]byte("PK\x03\x04"), // ZIP, XLSX, DOCX
	[]byte("PK\x05\x06"), // empty ZIP
	[]byte("PK\x07\x08"), // spanned ZIP
	[]byte("%PDF-"),
	[]byte("GIF87a"),
	[]byte("GIF89a"),
	[]byte("\xff\xd8\xff"),     // JPEG
	[]byte("\x1f\x8b"),         // gzip
	[]byte("\xd0\xcf\x11\xe0"), // legacy OLE (.xls)
	{0xFF, 0xFE},               // UTF-16 LE BOM
	{0xFE, 0xFF},               // UTF-16 BE BOM
}

// SniffImportText rejects an upload that is evidently binary, from its first
// 8 KiB, before any CSV parser sees it. It never rejects text the decoder
// reads: UTF-8 (with or without a BOM) and Windows-1252/Latin-1 text contain
// no NUL bytes and almost no C0 control bytes, and high bytes (0x80-0xFF) are
// not counted as control bytes. An empty file passes (the parser reports it).
// Rules, in order: a binary magic number; a NUL byte; more than 5% C0/DEL
// control bytes other than tab, LF, CR and form feed.
func SniffImportText(data []byte) error {
	for _, magic := range binaryMagics {
		if bytes.HasPrefix(data, magic) {
			return ErrImportNotText
		}
	}
	window := stripUTF8BOM(data)
	if len(window) > importSniffWindow {
		window = window[:importSniffWindow]
	}
	if len(window) == 0 {
		return nil
	}
	if bytes.IndexByte(window, 0) >= 0 {
		return ErrImportNotText
	}
	control := 0
	for _, b := range window {
		if (b < 0x20 && b != '\t' && b != '\n' && b != '\r' && b != '\f') || b == 0x7F {
			control++
		}
	}
	if control*20 > len(window) {
		return ErrImportNotText
	}
	return nil
}
