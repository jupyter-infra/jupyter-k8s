{{- define "type" -}}
{{- $type := . -}}
{{- if markdownShouldRenderType $type -}}

### {{ $type.Name }}

{{ if $type.IsAlias }}_Underlying type:_ _{{ markdownRenderTypeLink $type.UnderlyingType }}_{{ end }}

{{ $type.Doc }}

{{ if $type.Validation -}}
_Validation:_
{{- range $type.Validation }}
- {{ . }}
{{- end }}

{{ end -}}

{{ if $type.References -}}
_Appears in:_
{{- range $type.SortedReferences }}
- {{ markdownRenderTypeLink . }}
{{- end }}

{{ end -}}

{{ if $type.Members -}}
{{ if $type.GVK -}}
| Field | Value or Description |
| --- | --- |
| `apiVersion` _string_ | `{{ $type.GVK.Group }}/{{ $type.GVK.Version }}` |
| `kind` _string_ | `{{ $type.GVK.Kind }}` |
{{ range $type.Members -}}
| `{{ .Name }}` _{{ markdownRenderType .Type }}_ | {{ template "type_members" . }} |
{{ end -}}
{{ else -}}
| Field | Description | Default | Validation |
| --- | --- | --- | --- |
{{ range $type.Members -}}
| `{{ .Name }}` _{{ markdownRenderType .Type }}_ | {{ template "type_members" . }} | {{ markdownRenderDefault .Default }} | {{ range .Validation -}} {{ markdownRenderFieldDoc . }} <br />{{ end }} |
{{ end -}}
{{ end -}}

{{ end -}}

{{ if $type.EnumValues -}}
| Value | Description |
| --- | --- |
{{ range $type.EnumValues -}}
| `{{ .Name }}` | {{ markdownRenderFieldDoc .Doc }} |
{{ end -}}
{{ end -}}

{{- end -}}
{{- end -}}
