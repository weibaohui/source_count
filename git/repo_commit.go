// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"strconv"
	"strings"
	"time"
)

// parseCommit parses commit information from the (uncompressed) raw data of the commit object.
// It assumes "\n\n" separates the header from the rest of the message.
func parseCommit(data []byte) (*Commit, error) {
	commit := new(Commit)
	// we now have the contents of the commit object. Let's investigate.
	nextline := 0
loop:
	for {
		eol := bytes.IndexByte(data[nextline:], '\n')
		switch {
		case eol > 0:
			line := data[nextline : nextline+eol]
			spacepos := bytes.IndexByte(line, ' ')
			reftype := line[:spacepos]
			switch string(reftype) {
			// case "tree", "object":
			// 	id, err := NewIDFromString(string(line[spacepos+1:]))
			// 	if err != nil {
			// 		return nil, err
			// 	}
			// 	commit.Tree = &Tree{id: id}
			// case "parent":
			// 	// A commit can have one or more parents
			// 	id, err := NewIDFromString(string(line[spacepos+1:]))
			// 	if err != nil {
			// 		return nil, err
			// 	}
			// 	commit.parents = append(commit.parents, id)
			case "author", "tagger":
				sig, err := parseSignature(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Author = sig
			case "committer":
				sig, err := parseSignature(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Committer = sig
			}
			nextline += eol + 1
		case eol == 0:
			commit.Message = string(data[nextline+1:])
			break loop
		default:
			break loop
		}
	}
	return commit, nil
}

// CatFileCommitOptions contains optional arguments for verifying the objects.
// Docs: https://git-scm.com/docs/git-cat-file#Documentation/git-cat-file.txt-lttypegt
type CatFileCommitOptions struct {
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// CatFileCommit returns the commit corresponding to the given revision of the repository.
// The revision could be a commit ID or full refspec (e.g. "refs/heads/master").
func (r *Repository) CatFileCommit(rev string, opts ...CatFileCommitOptions) (*Commit, error) {
	var opt CatFileCommitOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	cache, ok := r.syncMapCachedCommits.Load(rev)
	if ok {
		return cache.(*Commit), nil
	}

	stdout, err := NewCommand("cat-file", "commit", rev).RunInDirWithTimeout(opt.Timeout, r.path)
	if err != nil {
		return nil, err
	}

	c, err := parseCommit(stdout)
	if err != nil {
		return nil, err
	}

	r.syncMapCachedCommits.Store(rev, c)
	return c, nil
}

// CatFileTypeOptions contains optional arguments for showing the object type.
// Docs: https://git-scm.com/docs/git-cat-file#Documentation/git-cat-file.txt--t
type CatFileTypeOptions struct {
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// CatFileType returns the object type of given revision of the repository.
func (r *Repository) CatFileType(rev string, opts ...CatFileTypeOptions) (string, error) {
	var opt CatFileTypeOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	typ, err := NewCommand("cat-file", "-t", rev).RunInDirWithTimeout(opt.Timeout, r.path)
	if err != nil {
		return "", err
	}
	typ = bytes.TrimSpace(typ)
	return string(typ), nil
}

// BranchCommit returns the latest commit of given branch of the repository.
// The branch must be given in short name e.g. "master".
func (r *Repository) BranchCommit(branch string, opts ...CatFileCommitOptions) (*Commit, error) {
	return r.CatFileCommit(RefsHeads+branch, opts...)
}

// TagCommit returns the latest commit of given tag of the repository.
// The tag must be given in short name e.g. "v1.0.0".
func (r *Repository) TagCommit(tag string, opts ...CatFileCommitOptions) (*Commit, error) {
	return r.CatFileCommit(RefsTags+tag, opts...)
}

// LogOptions contains optional arguments for listing commits.
// Docs: https://git-scm.com/docs/git-log
type LogOptions struct {
	// The maximum number of commits to output.
	MaxCount int
	// The number commits skipped before starting to show the commit output.
	Skip int
	// To only show commits since the time.
	Since time.Time
	// The regular expression to filter commits by their messages.
	GrepPattern string
	// Indicates whether to ignore letter case when match the regular expression.
	RegexpIgnoreCase bool
	// The relative path of the repository.
	Path string
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

func escapePath(path string) string {
	if len(path) == 0 {
		return path
	}

	// Path starts with ':' must be escaped.
	if path[0] == ':' {
		path = `\` + path
	}
	return path
}

func (r *Repository) SumAuthor(author *Signature) *AuthorLinesCounter {
	// fmt.Println("统计作者", author, time.Now())
	cmd := NewCommand("log")
	cmd.AddArgs("--author=" + author.Email)
	cmd.AddArgs("--pretty=tformat:")
	cmd.AddArgs("--numstat")
	stdout, err := cmd.RunInDirWithTimeout(DefaultTimeout, r.path)
	if err != nil {
		// fmt.Println(err.Error())
		Debug(err.Error())
		return nil
	}

	ac := &AuthorLinesCounter{
		Email:       author.Email,
		Name:        author.Name,
		Addition:    0,
		Deletion:    0,
		CommitCount: 0,
	}
	lines := bytes.Split(stdout, []byte{'\n'})
	ac.CommitCount = len(lines)
	for _, line := range lines {
		counts := bytes.Split(line, []byte{'\t'})
		if len(counts) >= 2 {
			// fmt.Printf("\n 序号%d,增加 %s ++ ,删除 %s --\n", i, string(counts[0]), string(counts[1]))
			add, _ := strconv.Atoi(string(counts[0]))
			deletion, _ := strconv.Atoi(string(counts[1]))
			ac.Addition += add
			ac.Deletion += deletion
		}
	}
	return ac
}

// Log returns a list of commits in the state of given revision of the repository.
// The returned list is in reverse chronological order.
func (r *Repository) Log(rev string, opts ...LogOptions) ([]*Commit, error) {
	var opt LogOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cmd := NewCommand("log", "--pretty="+LogFormatHashOnly, rev)
	if opt.MaxCount > 0 {
		cmd.AddArgs("--max-count=" + strconv.Itoa(opt.MaxCount))
	}
	if opt.Skip > 0 {
		cmd.AddArgs("--skip=" + strconv.Itoa(opt.Skip))
	}
	if !opt.Since.IsZero() {
		cmd.AddArgs("--since=" + opt.Since.Format(time.RFC3339))
	}
	if opt.GrepPattern != "" {
		cmd.AddArgs("--grep=" + opt.GrepPattern)
	}
	if opt.RegexpIgnoreCase {
		cmd.AddArgs("--regexp-ignore-case")
	}
	cmd.AddArgs("--")
	if opt.Path != "" {
		cmd.AddArgs(escapePath(opt.Path))
	}

	stdout, err := cmd.RunInDirWithTimeout(opt.Timeout, r.path)
	if err != nil {
		return nil, err
	}
	return r.parsePrettyFormatLogToList(opt.Timeout, stdout)
}

func (r *Repository) LogGo(rev string, opts ...LogOptions) (count int, err error) {
	var opt LogOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cmd := NewCommand("log", "--pretty="+LogFormatHashOnly, rev)
	if opt.MaxCount > 0 {
		cmd.AddArgs("--max-count=" + strconv.Itoa(opt.MaxCount))
	}
	if opt.Skip > 0 {
		cmd.AddArgs("--skip=" + strconv.Itoa(opt.Skip))
	}
	if !opt.Since.IsZero() {
		cmd.AddArgs("--since=" + opt.Since.Format(time.RFC3339))
	}
	if opt.GrepPattern != "" {
		cmd.AddArgs("--grep=" + opt.GrepPattern)
	}
	if opt.RegexpIgnoreCase {
		cmd.AddArgs("--regexp-ignore-case")
	}
	cmd.AddArgs("--")
	if opt.Path != "" {
		cmd.AddArgs(escapePath(opt.Path))
	}

	stdout, err := cmd.RunInDirWithTimeout(opt.Timeout, r.path)
	if err != nil {
		return 0, err
	}

	if len(stdout) == 0 {
		return 0, nil
	}
	ids := bytes.Split(stdout, []byte{'\n'})
	err = r.parsePrettyFormatLogToListGo(opt.Timeout, ids)

	return len(ids), err
}

// CommitByRevisionOptions contains optional arguments for getting a commit.
// Docs: https://git-scm.com/docs/git-log
type CommitByRevisionOptions struct {
	// The relative path of the repository.
	Path string
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// CommitByRevisionOptions returns a commit by given revision.
func (r *Repository) CommitByRevision(rev string, opts ...CommitByRevisionOptions) (*Commit, error) {
	var opt CommitByRevisionOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	commits, err := r.Log(rev, LogOptions{
		MaxCount: 1,
		Path:     opt.Path,
		Timeout:  opt.Timeout,
	})
	if err != nil {
		if strings.Contains(err.Error(), "bad revision") {
			return nil, ErrRevisionNotExist
		}
		return nil, err
	} else if len(commits) == 0 {
		return nil, ErrRevisionNotExist
	}
	return commits[0], nil
}

// CommitsByPageOptions contains optional arguments for getting paginated commits.
// Docs: https://git-scm.com/docs/git-log
type CommitsByPageOptions struct {
	// The relative path of the repository.
	Path string
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// CommitsByPage returns a paginated list of commits in the state of given revision.
// The pagination starts from the newest to the oldest commit.
func (r *Repository) CommitsByPage(rev string, page, size int, opts ...CommitsByPageOptions) ([]*Commit, error) {
	var opt CommitsByPageOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return r.Log(rev, LogOptions{
		MaxCount: size,
		Skip:     (page - 1) * size,
		Path:     opt.Path,
		Timeout:  opt.Timeout,
	})
}

// SearchCommitsOptions contains optional arguments for searching commits.
// Docs: https://git-scm.com/docs/git-log
type SearchCommitsOptions struct {
	// The maximum number of commits to output.
	MaxCount int
	// The relative path of the repository.
	Path string
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// SearchCommits searches commit message with given pattern in the state of given revision.
// The returned list is in reverse chronological order.
func (r *Repository) SearchCommits(rev, pattern string, opts ...SearchCommitsOptions) ([]*Commit, error) {
	var opt SearchCommitsOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return r.Log(rev, LogOptions{
		MaxCount:         opt.MaxCount,
		GrepPattern:      pattern,
		RegexpIgnoreCase: true,
		Path:             opt.Path,
		Timeout:          opt.Timeout,
	})
}

// CommitsSinceOptions contains optional arguments for listing commits since a time.
// Docs: https://git-scm.com/docs/git-log
type CommitsSinceOptions struct {
	// The relative path of the repository.
	Path string
	// The timeout duration before giving up for each shell command execution.
	// The default timeout duration will be used when not supplied.
	Timeout time.Duration
}

// CommitsSince returns a list of commits since given time. The returned list is in reverse
// chronological order.
func (r *Repository) CommitsSince(rev string, since time.Time, opts ...CommitsSinceOptions) ([]*Commit, error) {
	var opt CommitsSinceOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	return r.Log(rev, LogOptions{
		Since:   since,
		Path:    opt.Path,
		Timeout: opt.Timeout,
	})
}
