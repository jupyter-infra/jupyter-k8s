/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package posixidentity allocates per-user POSIX uids, recorded as PosixIdentity objects, and resolves
// the templated identity fields of a WorkspaceTemplate's security contexts.
package posixidentity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// Id space. Kubernetes accepts uids and gids in [0, 2^31). The operator owns the user range and the
// group range after it; the deployer picks team and volume gids anywhere else outside the reserved ids.
const (
	// UserRangeMin is the first uid of the user range, fixed by strategy v1
	UserRangeMin int64 = 1 << 20
	// UserRangeMax is the last uid of the user range, fixed by strategy v1
	UserRangeMax int64 = 1<<20 + 1<<30 - 1
	// GroupRangeMin is the first gid of the range reserved for operator-allocated group identities
	// (e.g. OIDC groups); nothing allocates from it yet, and deployer gids may not use it
	GroupRangeMin int64 = UserRangeMax + 1
	// GroupRangeMax is the last gid of the reserved group range (2^28 gids)
	GroupRangeMax int64 = GroupRangeMin + 1<<28 - 1
	// MinID is the lowest uid or gid a resolved identity may use; ids below are system ids
	MinID int64 = 1000
	// MaxID is the highest uid or gid Kubernetes accepts
	MaxID int64 = 1<<31 - 1
	// NobodyID is the overflow id (nobody / nogroup), never usable
	NobodyID int64 = 65534

	// FinalizerName protects a PosixIdentity: deleting one requires deliberately removing it
	FinalizerName = "workspace.jupyter.org/posix-identity-protection"

	// objectNamePrefix prefixes the uid in a PosixIdentity's name
	objectNamePrefix = "uid-"
	// maxProbes bounds the candidates tried for one allocation
	maxProbes = 64
	// maxUnixNameLength is the longest unix name derived from a username
	maxUnixNameLength = 32
)

// UsernameHash returns the lookup key of a username: the first 32 hex characters of its SHA-256 digest.
// The same function serves every PosixIdentity, whatever its allocation strategy.
func UsernameHash(username string) string {
	sum := sha256.Sum256([]byte(username))
	return hex.EncodeToString(sum[:16])
}

// UnixName derives the name of a user's passwd entry from their username: lowercase, every character outside [a-z0-9_-] replaced
// with '-', truncated to 32 characters. For example "github:JaneDoe" becomes "github-janedoe".
func UnixName(username string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(username) {
		if b.Len() == maxUnixNameLength {
			break
		}
		// Keep the portable POSIX user-name characters, lowercased (uppercase is portable but
		// conventionally avoided). '.' is portable too but replaced: chown's legacy user.group syntax
		// makes it awkward. Every other character becomes '-', which a username may also contain
		// literally, so the mapping is not reversible and two usernames can share a unix name: "jane@"
		// and "jane!" both become "jane-". That is harmless: they still get distinct uids (lookups go
		// by username), each workspace only resolves its owner's uid, so the other user's files show
		// as a bare uid, and ownership and permissions follow the uid, not the name. An admin who wants
		// distinct names pre-seeds one user's PosixIdentity with another unixName; the name is stored
		// there rather than recomputed.
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "user"
	}
	return b.String()
}

// ObjectName returns the name of the PosixIdentity recording a uid.
func ObjectName(uid int64) string {
	return objectNamePrefix + strconv.FormatInt(uid, 10)
}

// UIDOf returns the uid a PosixIdentity records in its name, uid-<uid>. A name that does not parse, or
// that records a reserved id, is an error for an admin to fix.
func UIDOf(identity *workspacev1alpha1.PosixIdentity) (int64, error) {
	digits, ok := strings.CutPrefix(identity.Name, objectNamePrefix)
	if !ok || digits == "" || digits[0] == '0' {
		return 0, fmt.Errorf("PosixIdentity %q: the name must be uid-<uid>", identity.Name)
	}
	uid, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || !ValidID(uid) {
		return 0, fmt.Errorf("PosixIdentity %q: the name must be uid-<uid> with a uid in [%d, %d], not %d", identity.Name, MinID, MaxID, NobodyID)
	}
	return uid, nil
}

// InUserRange reports whether an id lies in the operator's user range.
func InUserRange(id int64) bool {
	return id >= UserRangeMin && id <= UserRangeMax
}

// ValidID reports whether an id may be used by a resolved identity at all: not a system id, not the
// overflow id, and accepted by Kubernetes.
func ValidID(id int64) bool {
	return id >= MinID && id <= MaxID && id != NobodyID
}

// InGroupRange reports whether an id lies in the range reserved for operator-allocated groups.
func InGroupRange(id int64) bool {
	return id >= GroupRangeMin && id <= GroupRangeMax
}

// ValidGroupID reports whether a deployer-supplied gid (a namespace or volume gid) is usable: a valid
// id outside the operator's user and group ranges, so it never equals an operator-allocated id.
func ValidGroupID(id int64) bool {
	return ValidID(id) && !InUserRange(id) && !InGroupRange(id)
}

// v1Candidates returns the uids strategy v1 tries for a username, in order. Strategy v1 is frozen: its
// hash, range and probe sequence never change once released; a different algorithm is a new strategy.
// The start is the first 8 bytes of SHA-256(username) modulo the range size; probe i adds i*i, so
// concurrent requests for one user try the same sequence and converge on the same uid.
func v1Candidates(username string) []int64 {
	size := uint64(UserRangeMax - UserRangeMin + 1)
	sum := sha256.Sum256([]byte(username))
	start := binary.BigEndian.Uint64(sum[:8]) % size
	candidates := make([]int64, 0, maxProbes)
	for i := uint64(0); i < maxProbes; i++ {
		candidates = append(candidates, UserRangeMin+int64((start+i*i)%size))
	}
	return candidates
}
