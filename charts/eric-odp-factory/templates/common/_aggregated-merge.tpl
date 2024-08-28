{{- /*
Generic function for merging annotations and labels (version: 1.0.1)
{
    context: string
    sources: [[sourceData: {key => value}]]
}

This generic merge function is added to improve user experience
and help ADP services comply with the following design rules:
  - DR-D1121-060 (global labels and annotations)
  - DR-D1121-065 (annotations can be attached by application
                  developers, or by deployment engineers)
  - DR-D1121-068 (labels can be attached by application
                  developers, or by deployment engineers)
  - DR-D1121-160 (strings used as parameter value shall always
                  be quoted)

Installation or template generation of the Helm chart fails when:
  - same key is listed multiple times with different values
  - when the input is not string

IMPORTANT: This function is distributed between services verbatim.
Fixes and updates to this function will require services to reapply
this function to their codebase. Until usage of library charts is
supported in ADP, we will keep the function hardcoded here.
*/ -}}

{{- define "eric-odp-factory.aggregatedMerge" -}}
  {{- $merged := dict -}}
  {{- $context := .context -}}
  {{- $location := .location -}}

  {{- range $sourceData := .sources -}}
    {{- range $key, $value := $sourceData -}}

      {{- /* FAIL: when the input is not string. */ -}}
      {{- if not (kindIs "string" $value) -}}
        {{- $problem := printf "Failed to merge keys for \"%s\" in \"%s\": invalid type" $context $location -}}
        {{- $details := printf "in \"%s\": \"%s\"." $key $value -}}
        {{- $reason := printf "The merge function only accepts strings as input." -}}
        {{- $solution := "To proceed, please pass the value as a string and try again." -}}
        {{- printf "%s %s %s %s" $problem $details $reason $solution | fail -}}
      {{- end -}}

      {{- if hasKey $merged $key -}}
        {{- $mergedValue := index $merged $key -}}

        {{- /* FAIL: when there are different values for a key. */ -}}
        {{- if ne $mergedValue $value -}}
          {{- $problem := printf "Failed to merge keys for \"%s\" in \"%s\": key duplication in" $context $location -}}
          {{- $details := printf "\"%s\": (\"%s\", \"%s\")." $key $mergedValue $value -}}
          {{- $reason := printf "The same key cannot have different values." -}}
          {{- $solution := "To proceed, please resolve the conflict and try again." -}}
          {{- printf "%s %s %s %s" $problem $details $reason $solution | fail -}}
        {{- end -}}
      {{- end -}}

      {{- $_ := set $merged $key $value -}}

    {{- end -}}
  {{- end -}}

{{- /*
Strings used as parameter value shall always be quoted. (DR-D1121-160)

The below is a workaround to toYaml, which removes the quotes.
Instead we loop over and quote each value.
*/ -}}
{{- range $key, $value := $merged }}
{{ $key }}: {{ $value | quote }}
{{- end -}}
{{- end -}}

{{- /*
Wrapper functions to set the contexts
*/ -}}
{{- define "eric-odp-factory.mergeAnnotations" -}}
  {{- include "eric-odp-factory.aggregatedMerge" (dict "context" "annotations" "location" .location "sources" .sources) }}
{{- end -}}

{{- define "eric-odp-factory.mergeLabels" -}}
  {{- include "eric-odp-factory.aggregatedMerge" (dict "context" "labels" "location" .location "sources" .sources) }}
{{- end -}}
