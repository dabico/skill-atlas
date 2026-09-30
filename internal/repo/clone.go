package repo

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/skeema/knownhosts"
)

// Clone makes a depth-1 clone of t into dir, which must exist and be empty.
// An empty ref means the remote's default branch; otherwise ref is a branch or tag name.
// It never prompts.
func Clone(ctx context.Context, t Target, ref, dir string) (Checkout, error) {
	auth, err := authFor(t)
	if err != nil {
		return Checkout{}, err
	}

	name, err := resolveRef(ctx, t, auth, ref)
	if err != nil {
		return Checkout{}, err
	}

	opts := &git.CloneOptions{
		URL:               t.URL,
		Auth:              auth,
		SingleBranch:      true,
		Depth:             1,
		Tags:              git.NoTags,
		RecurseSubmodules: git.NoRecurseSubmodules,
	}
	if name != "" {
		opts.ReferenceName = name
	}
	r, err := git.PlainCloneContext(ctx, dir, false, opts)
	if err != nil {
		return Checkout{}, mapError(ctx, t, err)
	}

	head, err := r.Head()
	if err != nil {
		return Checkout{}, fmt.Errorf("read HEAD of %s: %w", t.Display, err)
	}
	short := name.Short()
	if name == "" {
		if !head.Name().IsBranch() {
			return Checkout{}, fmt.Errorf("can't determine the default branch of %s", t.Display)
		}
		short = head.Name().Short()
	}
	sha, err := commitSHA(r, head.Hash())
	if err != nil {
		return Checkout{}, fmt.Errorf("resolve commit of %s: %w", t.Display, err)
	}
	return Checkout{Ref: short, SHA: sha}, nil
}

// commitSHA peels annotated tag objects down to the commit.
func commitSHA(r *git.Repository, h plumbing.Hash) (string, error) {
	if tag, err := r.TagObject(h); err == nil {
		c, err := tag.Commit()
		if err != nil {
			return "", err
		}
		return c.Hash.String(), nil
	}
	return h.String(), nil
}

func authFor(t Target) (transport.AuthMethod, error) {
	if !t.SSH {
		return nil, nil
	}
	if os.Getenv("SSH_AUTH_SOCK") == "" {
		return nil, errors.New("SSH URLs need a running SSH agent (SSH_AUTH_SOCK isn't set)")
	}
	user := "git"
	if ep, err := transport.NewEndpoint(t.URL); err == nil && ep.User != "" {
		user = ep.User
	}
	// The nil HostKeyCallback falls back to ~/.ssh/known_hosts.
	auth, err := ssh.NewSSHAgentAuth(user)
	if err != nil {
		return nil, fmt.Errorf("connect to the SSH agent: %w", err)
	}
	return auth, nil
}

// resolveRef returns the full ref name to clone, or "" to let the clone pick the remote HEAD.
func resolveRef(ctx context.Context, t Target, auth transport.AuthMethod, ref string) (plumbing.ReferenceName, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{t.URL}})
	refs, err := rem.ListContext(ctx, &git.ListOptions{Auth: auth})
	if err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return "", fmt.Errorf("repository %s is empty", t.Display)
		}
		return "", mapError(ctx, t, err)
	}
	if len(refs) == 0 {
		return "", fmt.Errorf("repository %s is empty", t.Display)
	}

	if ref == "" {
		for _, r := range refs {
			if r.Name() == plumbing.HEAD && r.Type() == plumbing.SymbolicReference && r.Target().IsBranch() {
				return r.Target(), nil
			}
		}
		return "", nil
	}

	for _, want := range []plumbing.ReferenceName{plumbing.NewBranchReferenceName(ref), plumbing.NewTagReferenceName(ref)} {
		for _, r := range refs {
			if r.Name() == want {
				return want, nil
			}
		}
	}
	return "", fmt.Errorf("ref %q not found in %s", ref, t.Display)
}

func mapError(ctx context.Context, t Target, err error) error {
	host, _, _ := strings.Cut(t.Display, "/")
	var netErr net.Error
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("clone %s: %w", t.Display, ctx.Err())
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed),
		strings.Contains(err.Error(), "unable to authenticate"):
		msg := "authentication failed for " + t.Display
		if !t.SSH {
			msg += ": the repository may be private or may not exist"
		}
		return errors.New(msg)
	case errors.Is(err, transport.ErrRepositoryNotFound):
		return fmt.Errorf("repository %s not found", t.Display)
	case knownhosts.IsHostUnknown(err):
		return fmt.Errorf("unknown host key for %s, add it to ~/.ssh/known_hosts (e.g. ssh-keyscan %s >> ~/.ssh/known_hosts)", host, host)
	case knownhosts.IsHostKeyChanged(err):
		return fmt.Errorf("host key for %s doesn't match ~/.ssh/known_hosts", host)
	case errors.As(err, &netErr):
		return fmt.Errorf("can't reach %s: %w", host, err)
	}
	return fmt.Errorf("clone %s: %w", t.Display, err)
}
