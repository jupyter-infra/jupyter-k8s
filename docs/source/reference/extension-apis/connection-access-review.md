# ConnectionAccessReview

## ConnectionAccessReview



ConnectionAccessReview is the schema for ConnectionAccessReview API

| Field | Description |
| --- | --- |
| `apiVersion` _string_ | `connection.workspace.jupyter.org/v1alpha1` |
| `kind` _string_ | `ConnectionAccessReview` |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.35/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |
| `spec` _[ConnectionAccessReviewSpec](#connectionaccessreviewspec)_ |  |
| `status` _[ConnectionAccessReviewStatus](#connectionaccessreviewstatus)_ |  |



## ConnectionAccessReviewSpec



ConnectionAccessReviewSpec defines the parameters of the ConnectionAccessReview

_Appears in:_
- [ConnectionAccessReview](#connectionaccessreview)

| Field | Description |
| --- | --- |
| `workspaceName` _string_ |  |
| `groups` _string array_ |  |
| `uid` _string_ |  |
| `user` _string_ |  |
| `extra` _object (keys:string, values:string array)_ |  |



## ConnectionAccessReviewStatus



ConnectionAccessReviewStatus defines the observed state of the ConnectionAccessReview

_Appears in:_
- [ConnectionAccessReview](#connectionaccessreview)

| Field | Description |
| --- | --- |
| `allowed` _boolean_ |  |
| `notFound` _boolean_ |  |
| `reason` _string_ |  |


