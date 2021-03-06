package git

import (
	"time"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"
	"gopkg.in/src-d/go-git.v4/storage/memory"

	"bytes"
	. "gopkg.in/check.v1"
	"gopkg.in/src-d/go-billy.v4/memfs"
	"gopkg.in/src-d/go-billy.v4/osfs"
	"gopkg.in/src-d/go-billy.v4/util"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"
	"io/ioutil"
	"os"
	"os/exec"
)

func (s *WorktreeSuite) TestCommitInvalidOptions(c *C) {
	r, err := Init(memory.NewStorage(), memfs.New())
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	hash, err := w.Commit("", &CommitOptions{})
	c.Assert(err, Equals, ErrMissingAuthor)
	c.Assert(hash.IsZero(), Equals, true)
}

func (s *WorktreeSuite) TestCommitInitial(c *C) {
	expected := plumbing.NewHash("98c4ac7c29c913f7461eae06e024dc18e80d23a4")

	fs := memfs.New()
	storage := memory.NewStorage()

	r, err := Init(storage, fs)
	c.Assert(err, IsNil)

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	util.WriteFile(fs, "foo", []byte("foo"), 0644)

	_, err = w.Add("foo")
	c.Assert(err, IsNil)

	hash, err := w.Commit("foo\n", &CommitOptions{Author: defaultSignature()})
	c.Assert(hash, Equals, expected)
	c.Assert(err, IsNil)

	assertStorageStatus(c, r, 1, 1, 1, expected)
}

func (s *WorktreeSuite) TestCommitParent(c *C) {
	expected := plumbing.NewHash("ef3ca05477530b37f48564be33ddd48063fc7a22")

	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	util.WriteFile(fs, "foo", []byte("foo"), 0644)

	_, err = w.Add("foo")
	c.Assert(err, IsNil)

	hash, err := w.Commit("foo\n", &CommitOptions{Author: defaultSignature()})
	c.Assert(hash, Equals, expected)
	c.Assert(err, IsNil)

	assertStorageStatus(c, s.Repository, 13, 11, 10, expected)
}

func (s *WorktreeSuite) TestCommitAll(c *C) {
	expected := plumbing.NewHash("aede6f8c9c1c7ec9ca8d287c64b8ed151276fa28")

	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	util.WriteFile(fs, "LICENSE", []byte("foo"), 0644)
	util.WriteFile(fs, "foo", []byte("foo"), 0644)

	hash, err := w.Commit("foo\n", &CommitOptions{
		All:    true,
		Author: defaultSignature(),
	})

	c.Assert(hash, Equals, expected)
	c.Assert(err, IsNil)

	assertStorageStatus(c, s.Repository, 13, 11, 10, expected)
}

func (s *WorktreeSuite) TestRemoveAndCommitAll(c *C) {
	expected := plumbing.NewHash("907cd576c6ced2ecd3dab34a72bf9cf65944b9a9")

	fs := memfs.New()
	w := &Worktree{
		r:          s.Repository,
		Filesystem: fs,
	}

	err := w.Checkout(&CheckoutOptions{})
	c.Assert(err, IsNil)

	util.WriteFile(fs, "foo", []byte("foo"), 0644)
	_, err = w.Add("foo")
	c.Assert(err, IsNil)

	_, errFirst := w.Commit("Add in Repo\n", &CommitOptions{
		Author: defaultSignature(),
	})
	c.Assert(errFirst, IsNil)

	errRemove := fs.Remove("foo")
	c.Assert(errRemove, IsNil)

	hash, errSecond := w.Commit("Remove foo\n", &CommitOptions{
		All:    true,
		Author: defaultSignature(),
	})
	c.Assert(errSecond, IsNil)

	c.Assert(hash, Equals, expected)
	c.Assert(err, IsNil)

	assertStorageStatus(c, s.Repository, 13, 11, 11, expected)
}

func (s *WorktreeSuite) TestCommitTreeSort(c *C) {
	path, err := ioutil.TempDir(os.TempDir(), "test-commit-tree-sort")
	c.Assert(err, IsNil)
	fs := osfs.New(path)
	st, err := filesystem.NewStorage(fs)
	c.Assert(err, IsNil)
	r, err := Init(st, nil)
	c.Assert(err, IsNil)

	r, err = Clone(memory.NewStorage(), memfs.New(), &CloneOptions{
		URL: path,
	})

	w, err := r.Worktree()
	c.Assert(err, IsNil)

	mfs := w.Filesystem

	err = mfs.MkdirAll("delta", 0755)
	c.Assert(err, IsNil)

	for _, p := range []string{"delta_last", "Gamma", "delta/middle", "Beta", "delta-first", "alpha"} {
		util.WriteFile(mfs, p, []byte("foo"), 0644)
		_, err = w.Add(p)
		c.Assert(err, IsNil)
	}

	_, err = w.Commit("foo\n", &CommitOptions{
		All:    true,
		Author: defaultSignature(),
	})
	c.Assert(err, IsNil)

	err = r.Push(&PushOptions{})
	c.Assert(err, IsNil)

	cmd := exec.Command("git", "fsck")
	cmd.Dir = path
	cmd.Env = os.Environ()
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	cmd.Stdout = buf

	err = cmd.Run()

	c.Assert(err, IsNil, Commentf("%s", buf.Bytes()))
}

func assertStorageStatus(
	c *C, r *Repository,
	treesCount, blobCount, commitCount int, head plumbing.Hash,
) {
	trees, err := r.Storer.IterEncodedObjects(plumbing.TreeObject)
	c.Assert(err, IsNil)
	blobs, err := r.Storer.IterEncodedObjects(plumbing.BlobObject)
	c.Assert(err, IsNil)
	commits, err := r.Storer.IterEncodedObjects(plumbing.CommitObject)
	c.Assert(err, IsNil)

	c.Assert(lenIterEncodedObjects(trees), Equals, treesCount)
	c.Assert(lenIterEncodedObjects(blobs), Equals, blobCount)
	c.Assert(lenIterEncodedObjects(commits), Equals, commitCount)

	ref, err := r.Head()
	c.Assert(err, IsNil)
	c.Assert(ref.Hash(), Equals, head)
}

func lenIterEncodedObjects(iter storer.EncodedObjectIter) int {
	count := 0
	iter.ForEach(func(plumbing.EncodedObject) error {
		count++
		return nil
	})

	return count
}

func defaultSignature() *object.Signature {
	when, _ := time.Parse(object.DateFormat, "Thu May 04 00:03:43 2017 +0200")
	return &object.Signature{
		Name:  "foo",
		Email: "foo@foo.foo",
		When:  when,
	}
}
