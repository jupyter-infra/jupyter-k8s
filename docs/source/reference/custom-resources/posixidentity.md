# PosixIdentity

## PosixIdentity



PosixIdentity records the uid allocated to one user. It is an immutable ledger entry written by the
operator: one object per uid, named uid-<uid>, never garbage-collected since files on shared storage
keep carrying the uid after the user leaves.

| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `PosixIdentity` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[PosixIdentitySpec](#posixidentityspec)_ | spec is immutable once created |



## PosixIdentitySpec



PosixIdentitySpec is the immutable record of one allocated uid. The uid is the object's name,
uid-<uid>, so the API server's name uniqueness makes two records of one uid impossible.

_Appears in:_
- [PosixIdentity](#posixidentity)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `username` _string_ | Username is the Kubernetes username the uid belongs to, as in the workspace's created-by annotation.<br />The user's primary gid equals the uid (user private group). |  | MaxLength: 1024 <br />MinLength: 1 <br /> |
| `usernameHash` _string_ | UsernameHash is the lookup key of Username: the first 32 hex characters of its SHA-256 digest,<br />exposed to templates as \{\{ .User.UsernameHash \}\}. |  | Pattern: `^[0-9a-f]\{32\}$` <br /> |
| `unixName` _string_ | UnixName is the name of the user's passwd entry: what whoami prints for the uid inside the<br />workspace, and what ls -l shows as the owner of the user's files. It exists because a Kubernetes<br />username such as "github:JaneDoe" cannot be a passwd name (':' separates passwd fields).<br />The operator never writes passwd itself: templates read the name as \{\{ .User.UnixName \}\}, e.g. an<br />access strategy passing it to the image in an env var, and the image writes the passwd entry for<br />its uid (for instance with nss_wrapper).<br />The operator derives it from Username at allocation ("github:JaneDoe" becomes "github-janedoe");<br />an admin pre-seeding an object may set the name the user already has elsewhere, e.g. in a<br />directory. It is stored, so a change to the derivation rule never renames existing users. |  | Pattern: `^[a-z0-9_-]\{1,32\}$` <br /> |


