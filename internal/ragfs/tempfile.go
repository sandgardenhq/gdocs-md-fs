package ragfs

import "strings"

// isTempFile reports whether a filename matches known editor temp file patterns.
// These files are handled as ephemeral in-memory nodes and never synced to the backend.
func isTempFile(name string) bool {
	switch {
	case name == "4913": // vim writability test
		return true
	case strings.HasSuffix(name, "~"): // vim/emacs backup
		return true
	case strings.HasSuffix(name, ".swp"),
		strings.HasSuffix(name, ".swo"),
		strings.HasSuffix(name, ".swn"): // vim swap (may or may not have leading dot)
		return true
	case strings.HasSuffix(name, ".tmp"): // generic temp
		return true
	case strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#"): // emacs auto-save
		return true
	case strings.HasPrefix(name, "~$"): // MS Office lock
		return true
	case strings.HasPrefix(name, ".~lock."): // LibreOffice lock
		return true
	default:
		return false
	}
}
