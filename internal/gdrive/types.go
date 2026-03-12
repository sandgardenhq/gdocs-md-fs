// Package gdrive implements the ragfs.Handler interface backed by Google Drive
// and Google Docs APIs.
package gdrive

// Google Drive MIME type constants.
const (
	// MimeDoc is the MIME type for Google Docs documents.
	MimeDoc = "application/vnd.google-apps.document"
	// MimeFolder is the MIME type for Google Drive folders.
	MimeFolder = "application/vnd.google-apps.folder"
	// MimePDF is the MIME type for PDF files.
	MimePDF = "application/pdf"
	// MimeSheet is the MIME type for Google Sheets spreadsheets.
	MimeSheet = "application/vnd.google-apps.spreadsheet"
	// MimeSlides is the MIME type for Google Slides presentations.
	MimeSlides = "application/vnd.google-apps.presentation"
	// MimePlainText is the MIME type for plain text export.
	MimePlainText = "text/plain"
	// MimeMarkdown is the MIME type for markdown text.
	MimeMarkdown = "text/markdown"
)

// pathEntry maps a filesystem path to a Google Drive file ID and its MIME type.
type pathEntry struct {
	fileID   string
	mimeType string
	parentID string
	name     string
}
