package git

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/svagner/go-git.v4.1/config"
	"gopkg.in/svagner/go-git.v4.1/plumbing"
	"gopkg.in/svagner/go-git.v4.1/plumbing/client"
	"gopkg.in/svagner/go-git.v4.1/plumbing/client/common"
	"gopkg.in/svagner/go-git.v4.1/plumbing/format/packfile"
	"gopkg.in/svagner/go-git.v4.1/plumbing/format/packp"
	"gopkg.in/svagner/go-git.v4.1/plumbing/storer"
)

var NoErrAlreadyUpToDate = errors.New("already up-to-date")

// Remote represents a connection to a remote repository
type Remote struct {
	c *config.RemoteConfig
	s Storer

	// cache fields, there during the connection is open
	upSrv  common.GitUploadPackService
	upInfo *common.GitUploadPackInfo
}

func newRemote(s Storer, c *config.RemoteConfig) *Remote {
	return &Remote{s: s, c: c}
}

// Config return the config
func (r *Remote) Config() *config.RemoteConfig {
	return r.c
}

// Connect with the endpoint
func (r *Remote) Connect(auth common.AuthMethod) error {
	if err := r.connectUploadPackService(auth); err != nil {
		return err
	}

	return r.retrieveUpInfo()
}

func (r *Remote) connectUploadPackService(auth common.AuthMethod) error {
	endpoint, err := common.NewEndpoint(r.c.URL)
	if err != nil {
		return err
	}

	r.upSrv, err = clients.NewGitUploadPackService(endpoint)
	if err != nil {
		return err
	}
	r.upSrv.SetAuth(auth)
	return r.upSrv.Connect()
}

func (r *Remote) retrieveUpInfo() error {
	var err error
	if r.upInfo, err = r.upSrv.Info(); err != nil {
		return err
	}

	return nil
}

// Info returns the git-upload-pack info
func (r *Remote) Info() *common.GitUploadPackInfo {
	return r.upInfo
}

// Capabilities returns the remote capabilities
func (r *Remote) Capabilities() *packp.Capabilities {
	return r.upInfo.Capabilities
}

// Fetch returns a reader using the request
func (r *Remote) Fetch(o *FetchOptions) (err error) {
	if err := o.Validate(); err != nil {
		return err
	}

	if len(o.RefSpecs) == 0 {
		o.RefSpecs = r.c.Fetch
	}

	refs, err := r.getWantedReferences(o.RefSpecs)
	if err != nil {
		return err
	}

	if len(refs) == 0 {
		return NoErrAlreadyUpToDate
	}

	req, err := r.buildRequest(r.s, o, refs)
	if err != nil {
		return err
	}

	reader, err := r.upSrv.Fetch(req)
	if err != nil {
		return err
	}

	defer checkClose(reader, &err)
	if err := r.updateObjectStorage(reader); err != nil {
		return err
	}

	return r.updateLocalReferenceStorage(o.RefSpecs, refs)
}

func (r *Remote) getWantedReferences(spec []config.RefSpec) ([]*plumbing.Reference, error) {
	var refs []*plumbing.Reference
	iter, err := r.Refs()
	if err != nil {
		return refs, err
	}

	wantTags := true
	for _, s := range spec {
		if !s.IsWildcard() {
			wantTags = false
			break
		}
	}

	return refs, iter.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}

		if !config.MatchAny(spec, ref.Name()) {
			if !ref.IsTag() || !wantTags {
				return nil
			}
		}

		_, err := r.s.Object(plumbing.CommitObject, ref.Hash())
		if err == plumbing.ErrObjectNotFound {
			refs = append(refs, ref)
			return nil
		}

		return err
	})
}

func (r *Remote) buildRequest(
	s storer.ReferenceStorer, o *FetchOptions, refs []*plumbing.Reference,
) (*common.GitUploadPackRequest, error) {
	req := &common.GitUploadPackRequest{}
	req.Depth = o.Depth

	for _, ref := range refs {
		req.Want(ref.Hash())
	}

	i, err := s.IterReferences()
	if err != nil {
		return nil, err
	}

	err = i.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}

		req.Have(ref.Hash())
		return nil
	})

	return req, err
}

func (r *Remote) updateObjectStorage(reader io.Reader) error {
	if sw, ok := r.s.(storer.PackfileWriter); ok {
		w, err := sw.PackfileWriter()
		if err != nil {
			return err
		}

		defer w.Close()
		_, err = io.Copy(w, reader)
		return err
	}

	stream := packfile.NewScanner(reader)
	d, err := packfile.NewDecoder(stream, r.s)
	if err != nil {
		return err
	}

	_, err = d.Decode()
	return err
}

func (r *Remote) updateLocalReferenceStorage(specs []config.RefSpec, refs []*plumbing.Reference) error {
	for _, spec := range specs {
		for _, ref := range refs {
			if !spec.Match(ref.Name()) {
				continue
			}

			if ref.Type() != plumbing.HashReference {
				continue
			}

			name := spec.Dst(ref.Name())
			n := plumbing.NewHashReference(name, ref.Hash())
			if err := r.s.SetReference(n); err != nil {
				return err
			}
		}
	}

	return r.buildFetchedTags()
}

func (r *Remote) buildFetchedTags() error {
	iter, err := r.Refs()
	if err != nil {
		return err
	}

	return iter.ForEach(func(ref *plumbing.Reference) error {
		if !ref.IsTag() {
			return nil
		}

		_, err := r.s.Object(plumbing.AnyObject, ref.Hash())
		if err == plumbing.ErrObjectNotFound {
			return nil
		}

		if err != nil {
			return err
		}

		return r.s.SetReference(ref)
	})
}

// Head returns the Reference of the HEAD
func (r *Remote) Head() *plumbing.Reference {
	return r.upInfo.Head()
}

// Ref returns the Hash pointing the given refName
func (r *Remote) Ref(name plumbing.ReferenceName, resolved bool) (*plumbing.Reference, error) {
	if resolved {
		return storer.ResolveReference(r.upInfo.Refs, name)
	}

	return r.upInfo.Refs.Reference(name)
}

// Refs returns a map with all the References
func (r *Remote) Refs() (storer.ReferenceIter, error) {
	return r.upInfo.Refs.IterReferences()
}

// Disconnect from the remote and save the config
func (r *Remote) Disconnect() error {
	r.upInfo = nil
	return r.upSrv.Disconnect()
}

func (r *Remote) String() string {
	fetch := r.c.URL
	push := r.c.URL

	return fmt.Sprintf("%s\t%s (fetch)\n%[1]s\t%s (push)", r.c.Name, fetch, push)
}
