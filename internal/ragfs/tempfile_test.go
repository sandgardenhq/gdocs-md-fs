package ragfs

import "testing"

func TestIsTempFile(t *testing.T) {
	temps := []string{
		"doc.md~",         // vim/emacs backup
		".doc.md.swp",     // vim swap
		".doc.md.swo",     // vim swap overflow
		".doc.md.swn",     // vim swap overflow
		"notes.tmp",       // generic temp
		"#autosave#",      // emacs auto-save
		"~$document.docx", // MS Office lock
		".~lock.file.ods", // LibreOffice lock
		"4913",            // vim writability test
	}
	for _, name := range temps {
		if !isTempFile(name) {
			t.Errorf("isTempFile(%q) = false, want true", name)
		}
	}
}

func TestIsTempFile_NonTemp(t *testing.T) {
	nonTemps := []string{
		"readme.md",
		"notes.txt",
		"photo.png",
		"report.pdf",
		"my-file.md",
		".hidden",
		"backup",
		"49130",     // not exactly "4913"
		"a4913",     // not exactly "4913"
	}
	for _, name := range nonTemps {
		if isTempFile(name) {
			t.Errorf("isTempFile(%q) = true, want false", name)
		}
	}
}
