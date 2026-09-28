package rename

import "path/filepath"

// ApplyResult is what Apply did. A failed rename leaves that file as it was.
type ApplyResult struct {
	Renamed  int
	Checked  int    // of Renamed, how many used check-then-rename rather than an atomic no-replace rename
	Failures []Skip // Name is the old name
}

// Apply renames every file in the plan. One failure does not stop the rest.
// On a volume with RENAME_EXCL support, each rename is atomic: the kernel
// itself refuses to replace anything created at the new name after the plan
// was made. Elsewhere, each target is instead checked for a collision just
// before renaming (see checkedRename), which leaves a small window open.
func Apply(p *RenamePlan) *ApplyResult {
	res := &ApplyResult{}
	for _, r := range p.Renames {
		checked, err := renameNoReplace(filepath.Join(p.Dir, r.Old), filepath.Join(p.Dir, r.New))
		if err != nil {
			res.Failures = append(res.Failures, Skip{Name: r.Old, Reason: err.Error()})
			continue
		}
		res.Renamed++
		if checked {
			res.Checked++
		}
	}
	return res
}
