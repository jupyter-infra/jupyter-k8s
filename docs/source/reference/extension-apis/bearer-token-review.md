# BearerTokenReview

## BearerTokenReview



BearerTokenReview is the schema for BearerTokenReview API

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `connection.workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `BearerTokenReview` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[BearerTokenReviewSpec](#bearertokenreviewspec)_ |  |
| `status` _[BearerTokenReviewStatus](#bearertokenreviewstatus)_ |  |



## BearerTokenReviewSpec



BearerTokenReviewSpec defines the parameters of the BearerTokenReview

_Appears in:_
- [BearerTokenReview](#bearertokenreview)

| Field | Description |
| --- | --- |
| `token` _string_ |  |



## BearerTokenReviewStatus



BearerTokenReviewStatus defines the result of the BearerTokenReview

_Appears in:_
- [BearerTokenReview](#bearertokenreview)

| Field | Description |
| --- | --- |
| `authenticated` _boolean_ |  |
| `path` _string_ |  |
| `domain` _string_ |  |
| `user` _[BearerTokenReviewUser](#bearertokenreviewuser)_ |  |
| `error` _string_ |  |



## BearerTokenReviewUser



BearerTokenReviewUser holds the identity extracted from a verified bearer token

_Appears in:_
- [BearerTokenReviewStatus](#bearertokenreviewstatus)

| Field | Description |
| --- | --- |
| `username` _string_ |  |
| `uid` _string_ |  |
| `groups` _string array_ |  |
| `extra` _object (keys:string, values:string array)_ |  |


