// adopt.go ports SkillLibrary::adopt (src/library.rs): adopt one scanned
// source and its observed links as a single recoverable operation.
// Source files and native plugin registries are never written here.
package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/saltand/kitter/native/core/adoption"
	"github.com/saltand/kitter/native/core/fslink"
	"github.com/saltand/kitter/native/core/model"
	"github.com/saltand/kitter/native/core/project"
	"github.com/saltand/kitter/native/core/skillfile"
)

// Adopt is SkillLibrary::adopt. candidate was produced by
// adoption.Scan/ScanRoots; references are the links to repoint at the
// candidate's source (scan.ReferencesFor(candidate)).
func (l *SkillLibrary) Adopt(candidate *adoption.AdoptionCandidate, references []model.SkillReference) (string, error) {
	if err := candidate.Verify(); err != nil {
		return "", err
	}
	if candidate.Issue != "" {
		return "", errors.New(candidate.Issue)
	}
	if err := skillfile.ValidateName(candidate.Name); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	var refs []model.SkillReference
	for _, r := range references {
		if seen[project.InstallationKey(r.Path)] {
			continue
		}
		seen[project.InstallationKey(r.Path)] = true
		refs = append(refs, r)
	}
	for i := range refs {
		if err := adoption.ValidateReference(&refs[i]); err != nil {
			return "", err
		}
	}
	before := l.Registry
	beforeBytes, _ := registrySnapshot(before)
	identity := candidate.Identity()
	var existing *model.SkillRecord
	for _, record := range l.Registry.Skills {
		if record.IdentityKey() == identity {
			copy := record
			existing = &copy
			break
		}
	}
	storage := ""
	if existing != nil {
		storage = existing.StorageName
	} else {
		storage = l.storageNameFor(candidate.Name, identity)
	}
	destination := filepath.Join(l.Config.LibraryDir, storage)
	alreadyPointsHere := false
	if resolved, err := filepath.EvalSymlinks(destination); err == nil {
		if abs, err2 := filepath.Abs(resolved); err2 == nil && abs == candidate.Source {
			alreadyPointsHere = true
		}
	}
	var backup string
	created := false
	var changed []model.SkillReference
	err := func() error {
		// Resolve all snapshots before changing anything. A link can
		// point through another reference or through the library entry
		// that this operation switches.
		for i := range refs {
			reference := &refs[i]
			if reference.Kind != model.ReferenceLink {
				continue
			}
			if project.InstallationKey(reference.Path) == candidate.Source {
				continue
			}
			current, err := fslink.Inspect(reference.Path)
			if err != nil || current == nil {
				return errors.New("引用已变化，请重新扫描")
			}
			if reference.OriginalTarget == nil || current.Target != *reference.OriginalTarget {
				return fmt.Errorf("引用已变化，请重新扫描：%s", reference.Path)
			}
			if reference.OriginalTarget == nil || *reference.OriginalTarget != candidate.Source {
				if err := adoption.ReplaceLink(reference.Path, candidate.Source); err != nil {
					return err
				}
				changed = append(changed, *reference)
			}
			resolved, err := filepath.EvalSymlinks(reference.Path)
			if err != nil {
				return err
			}
			if abs, err := filepath.Abs(resolved); err != nil || abs != candidate.Source {
				return fmt.Errorf("引用验证失败：%s", reference.Path)
			}
		}
		if !alreadyPointsHere {
			if _, err := os.Lstat(destination); err == nil {
				// Keep an existing Kitter materialization recoverable,
				// never overwrite it.
				parent := filepath.Join(l.Config.LibraryDir, ".adoption-backups")
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return err
				}
				dir, err := os.MkdirTemp(parent, "source-")
				if err != nil {
					return err
				}
				old := filepath.Join(dir, storage)
				if err := os.Rename(destination, old); err != nil {
					return err
				}
				backup = old
			}
			if err := fslink.Create(candidate.Source, destination); err != nil {
				return err
			}
			created = true
		}
		record := candidate.Record()
		record.StorageName = storage
		if existing != nil {
			record.GroupID = existing.GroupID
			record.KitterManual = existing.KitterManual
		}
		record.LastOperatedAt = operationStamp()
		source := record.Origin.Source()
		sourceRecord, ok := l.Registry.Sources[source.Key()]
		if !ok {
			sourceRecord = model.SkillSourceRecord{
				Source:           source,
				DiscoveredSkills: []string{},
				AddedSkills:      []string{},
			}
		}
		for _, list := range []*[]string{&sourceRecord.DiscoveredSkills, &sourceRecord.AddedSkills} {
			if !contains(*list, record.Name) {
				*list = append(*list, record.Name)
				sort.Strings(*list)
			}
		}
		l.Registry.Sources[source.Key()] = sourceRecord
		l.Registry.Skills[storage] = record
		var owned []model.SkillReference
		if prev, ok := l.Registry.AdoptedSources[storage]; ok {
			owned = prev.References
		}
		for _, reference := range refs {
			if reference.Kind != model.ReferenceLink {
				continue
			}
			reference.Source = candidate.Source
			reference.OriginalTarget = &candidate.Source
			var kept []model.SkillReference
			for _, r := range owned {
				if project.InstallationKey(r.Path) != project.InstallationKey(reference.Path) {
					kept = append(kept, r)
				}
			}
			owned = append(kept, reference)
		}
		previousLibrary := backup
		if previousLibrary == "" {
			if prev, ok := l.Registry.AdoptedSources[storage]; ok && prev.PreviousLibrary != nil {
				previousLibrary = *prev.PreviousLibrary
			}
		}
		entry := AdoptedSource{
			Source:     candidate.Source,
			References: owned,
		}
		if previousLibrary != "" {
			entry.PreviousLibrary = &previousLibrary
		}
		l.Registry.AdoptedSources[storage] = entry
		return l.Save()
	}()
	if err != nil {
		failures := adoption.RollbackLinks(changed)
		if created {
			if dl, inspectErr := fslink.Inspect(destination); inspectErr == nil && dl != nil {
				if removeErr := fslink.Remove(destination, *dl); removeErr != nil {
					failures = append(failures, removeErr.Error())
				}
			}
		}
		if backup != "" {
			if renameErr := os.Rename(backup, destination); renameErr != nil {
				failures = append(failures, renameErr.Error())
			}
		}
		l.restoreRegistry(beforeBytes)
		if len(failures) == 0 {
			return "", err
		}
		return "", fmt.Errorf("%s；回滚未完成：%s", err, strings.Join(failures, "；"))
	}
	return storage, nil
}

// registrySnapshot marshals the registry for restoreRegistry.
func registrySnapshot(r *Registry) ([]byte, error) {
	return r.MarshalJSON()
}

// restoreRegistry replaces the live registry after a failed adopt.
func (l *SkillLibrary) restoreRegistry(bytes []byte) {
	if bytes == nil {
		return
	}
	var r Registry
	if err := r.UnmarshalJSON(bytes); err == nil {
		l.Registry = &r
	}
}
