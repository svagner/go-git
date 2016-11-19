package filesystem

import (
	"gopkg.in/svagner/go-git.v4.1/plumbing"
	"gopkg.in/svagner/go-git.v4.1/plumbing/storer"
	"gopkg.in/svagner/go-git.v4.1/storage/filesystem/internal/dotgit"
)

type ReferenceStorage struct {
	dir *dotgit.DotGit
}

func (r *ReferenceStorage) SetReference(ref *plumbing.Reference) error {
	return r.dir.SetRef(ref)
}

func (r *ReferenceStorage) Reference(n plumbing.ReferenceName) (*plumbing.Reference, error) {
	return r.dir.Ref(n)
}

func (r *ReferenceStorage) IterReferences() (storer.ReferenceIter, error) {
	refs, err := r.dir.Refs()
	if err != nil {
		return nil, err
	}

	return storer.NewReferenceSliceIter(refs), nil
}
