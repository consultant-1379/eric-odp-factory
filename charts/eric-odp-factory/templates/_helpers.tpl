{{/*
Expand the name of the chart.
*/}}
{{- define "eric-odp-factory.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "eric-odp-factory.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "eric-odp-factory.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Kubernetes labels
*/}}
{{- define "eric-odp-factory.kubernetes-labels" -}}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ include "eric-odp-factory.chart" . }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "eric-odp-factory.labels" -}}
  {{- $kubernetesLabels := include "eric-odp-factory.kubernetes-labels" . | fromYaml -}}
  {{- $selectorLabels := include "eric-odp-factory.selectorLabels" . | fromYaml -}}
  {{- $globalLabels := (.Values.global).labels -}}
  {{- $serviceLabels := .Values.labels -}}
{{- include "eric-odp-factory.mergeLabels" (dict "location" .Template.Name "sources" (list $kubernetesLabels $selectorLabels $globalLabels $serviceLabels)) }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "eric-odp-factory.selectorLabels" -}}
app.kubernetes.io/name: {{ include "eric-odp-factory.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create a map from ".Values.global" with defaults if missing in values file.
This hides defaults from values file.
*/}}
{{ define "eric-odp-factory.global" }}
  {{- $globalDefaults := dict "security" (dict "tls" (dict "enabled" true)) -}}
  {{- $globalDefaults := merge $globalDefaults (dict "nodeSelector" (dict)) -}}
  {{- $globalDefaults := merge $globalDefaults (dict "registry" (dict "url" "armdocker.rnd.ericsson.se")) -}}
  {{- $globalDefaults := merge $globalDefaults (dict "pullSecret" "") -}}
  {{- $globalDefaults := merge $globalDefaults (dict "timezone" "UTC") -}}
  {{- $globalDefaults := merge $globalDefaults (dict "externalIPv4" (dict "enabled")) -}}
  {{- $globalDefaults := merge $globalDefaults (dict "externalIPv6" (dict "enabled")) -}}
  {{- $globalDefaults := merge $globalDefaults (dict "log" (dict "outputs" (list  "k8sLevel"))) -}}
  {{ if .Values.global }}
    {{- mergeOverwrite $globalDefaults .Values.global | toJson -}}
  {{ else }}
    {{- $globalDefaults | toJson -}}
  {{ end }}
{{ end }}

{{/*
Create a user defined annotation (DR-D1121-065, DR-D1121-060)
*/}}
{{ define "eric-odp-factory.config-annotations" }}
  {{- $global := (.Values.global).annotations -}}
  {{- $service := .Values.annotations -}}
  {{- include "eric-odp-factory.mergeAnnotations" (dict "location" .Template.Name "sources" (list $global $service)) -}}
{{- end }}

{{/*
Define the common annotations
*/}}
{{- define "eric-odp-factory.annotations" -}}
{{- $productInfo := include "eric-odp-factory.product-info" . | fromYaml -}}
{{- $config := include "eric-odp-factory.config-annotations" . | fromYaml -}}
{{- include "eric-odp-factory.mergeAnnotations" (dict "location" .Template.Name "sources" (list $productInfo $config)) }}
{{- end -}}

{{/*
Define annotations on the Service adding any specific ones for the service
*/}}
{{- define "eric-odp-factory.annotations-service" -}}
  {{- if .Values.service.annotations }}
    {{- $commonAnn := include "eric-odp-factory.annotations" . | fromYaml -}}
    {{- $serviceAnn := .Values.service.annotations }}
    {{- include "eric-odp-factory.mergeAnnotations" (dict "location" .Template.Name "sources" (list $commonAnn $serviceAnn)) }}
  {{- else -}}
    {{- template "eric-odp-factory.annotations" . }}
  {{- end }}
{{- end }}


{{/*
Annotations with Prometheus
*/}}
{{- define "eric-odp-factory.annotations-with-prometheus" -}}
  {{- $annotations := include "eric-odp-factory.annotations" . | fromYaml -}}
  {{- $prometheus := include "eric-odp-factory.prometheus" . | fromYaml -}}
  {{- include "eric-odp-factory.mergeAnnotations" (dict "location" .Template.Name "sources" (list $annotations $prometheus)) }}
{{- end -}}


{{/*
Create prometheus info
*/}}
{{- define "eric-odp-factory.prometheus" }}
prometheus.io/port: {{ .Values.container.ports.metrics | quote }}
prometheus.io/path: "/metrics"
{{- range $k, $v := .Values.prometheus }}
prometheus.io/{{ $k | replace "_" "-" }} : {{ $v | quote }}
{{- end }}
{{- end }}

{{/*
Create Ericsson product specific annotations
*/}}
{{- define "eric-odp-factory.helm-annotations_product_name" -}}
{{- $productname := (fromYaml (.Files.Get "eric-product-info.yaml")).productName -}}
{{- print $productname | quote }}
{{- end -}}
{{- define "eric-odp-factory.helm-annotations_product_number" -}}
{{- $productNumber := (fromYaml (.Files.Get "eric-product-info.yaml")).productNumber -}}
{{- print $productNumber | quote }}
{{- end -}}
{{- define "eric-odp-factory.helm-annotations_product_revision" -}}
{{- $ddbMajorVersion := mustRegexFind "^([0-9]+)\\.([0-9]+)\\.([0-9]+)((-|\\+)EP[0-9]+)*((-|\\+)[0-9]+)*" .Chart.Version -}}
{{- print $ddbMajorVersion | quote }}
{{- end -}}

{{/*
Create a dict of annotations for the product information (DR-D1121-064, DR-D1121-067).
*/}}
{{- define "eric-odp-factory.product-info" }}
ericsson.com/product-name: {{ template "eric-odp-factory.helm-annotations_product_name" . }}
ericsson.com/product-number: {{ template "eric-odp-factory.helm-annotations_product_number" . }}
ericsson.com/product-revision: {{ template "eric-odp-factory.helm-annotations_product_revision" . }}
{{- end }}

{{/*
Create a merged set of nodeSelectors from global and service level.
*/}}
{{ define "eric-odp-factory.nodeSelector" }}
  {{- $g := fromJson (include "eric-odp-factory.global" .) -}}
  {{- if .Values.nodeSelector -}}
    {{- range $key, $localValue := .Values.nodeSelector -}}
      {{- if hasKey $g.nodeSelector $key -}}
          {{- $globalValue := index $g.nodeSelector $key -}}
          {{- if ne $globalValue $localValue -}}
            {{- printf "nodeSelector \"%s\" is specified in both global (%s: %s) and service level (%s: %s) with differing values which is not allowed." $key $key $globalValue $key $localValue | fail -}}
          {{- end -}}
      {{- end -}}
    {{- end -}}
    {{- toYaml (merge $g.nodeSelector .Values.nodeSelector) | trim -}}
  {{- else -}}
    {{- toYaml $g.nodeSelector | trim -}}
  {{- end -}}
{{ end }}

{{/*
Create image pull secrets
*/}}
{{- define "eric-odp-factory.pullSecrets" -}}
{{- $g := fromJson (include "eric-odp-factory.global" .) -}}
{{- $pullSecret := $g.pullSecret -}}
{{- if .Values.imageCredentials -}}
    {{- if .Values.imageCredentials.pullSecret -}}
        {{- $pullSecret = .Values.imageCredentials.pullSecret -}}
    {{- end -}}
{{- end -}}
{{- print $pullSecret -}}
{{- end -}}

{{/*
The image path (DR-D1121-067, DR-D1121-104, DR-D1121-105, DR-D1121-106)
*/}}
{{- define "eric-odp-factory.mainImagePath" }}
    {{- $productInfo := fromYaml (.Files.Get "eric-product-info.yaml") -}}
    {{- $registryUrl := $productInfo.images.factory.registry -}}
    {{- $repoPath := $productInfo.images.factory.repoPath -}}
    {{- $name := $productInfo.images.factory.name -}}
    {{- $tag := $productInfo.images.factory.tag -}}
    {{- if ((.Values).global).registry -}}
        {{- if .Values.global.registry.url -}}
            {{- $registryUrl = .Values.global.registry.url -}}
        {{- end -}}
        {{- if not (kindIs "invalid" .Values.global.registry.repoPath) -}}
            {{- $repoPath = .Values.global.registry.repoPath -}}
        {{- end -}}
    {{- end -}}
    {{- if .Values.imageCredentials -}}
        {{- if (((.Values).imageCredentials).registry).url -}}
            {{- $registryUrl = .Values.imageCredentials.registry.url -}}
        {{- end -}}
        {{- if not (kindIs "invalid" .Values.imageCredentials.repoPath) -}}
            {{- $repoPath = .Values.imageCredentials.repoPath -}}
        {{- end -}}
        {{- if .Values.imageCredentials.factory -}}
            {{- if ((((.Values).imageCredentials).factory).registry).url -}}
                {{- $registryUrl = .Values.imageCredentials.factory.registry.url -}}
            {{- end -}}
            {{- if not (kindIs "invalid" .Values.imageCredentials.factory.repoPath) -}}
                {{- $repoPath = .Values.imageCredentials.factory.repoPath -}}
            {{- end -}}
        {{- end -}}
    {{- end -}}
    {{- if $repoPath -}}
        {{- $repoPath = printf "%s/" $repoPath -}}
    {{- end -}}
    {{- printf "%s/%s%s:%s" $registryUrl $repoPath $name $tag -}}
{{- end -}}

{{/*
Create pull policy
*/}}
{{- define "eric-odp-factory.imagePullPolicy" -}}
{{- $globalRegistryPullPolicy := "IfNotPresent" -}}
    {{- if .Values.imageCredentials.factory -}}
        {{- if .Values.imageCredentials.factory.registry -}}
            {{- if .Values.imageCredentials.factory.registry.imagePullPolicy -}}
                 {{- $globalRegistryPullPolicy = .Values.imageCredentials.factory.registry.imagePullPolicy -}}
            {{- end -}}
        {{- else if .Values.imageCredentials.pullPolicy -}}
            {{- $globalRegistryPullPolicy = .Values.imageCredentials.pullPolicy -}}
        {{- else if .Values.global -}}
            {{- if .Values.global.registry -}}
                {{- if .Values.global.registry.imagePullPolicy -}}
                    {{- $globalRegistryPullPolicy = .Values.global.registry.imagePullPolicy -}}
                {{- end -}}
            {{- end -}}
        {{- end -}}
    {{- else if .Values.global -}}
        {{- if .Values.global.registry -}}
            {{- if .Values.global.registry.imagePullPolicy -}}
                {{- $globalRegistryPullPolicy = .Values.global.registry.imagePullPolicy -}}
            {{- end -}}
        {{- end -}}
    {{- end -}}
    {{- print $globalRegistryPullPolicy -}}
{{- end -}}

{{/*
Define RoleBinding value
*/}}
{{- define "eric-odp-factory.roleBinding" -}}
{{- $rolebinding := false -}}
{{- if .Values.global -}}
    {{- if .Values.global.security -}}
        {{- if .Values.global.security.policyBinding -}}
            {{- if hasKey .Values.global.security.policyBinding "create" -}}
                {{- $rolebinding = .Values.global.security.policyBinding.create -}}
            {{- end -}}
        {{- end -}}
    {{- end -}}
{{- end -}}
{{- $rolebinding -}}
{{- end -}}

{{/*
Define reference to SecurityPolicy
*/}}
{{- define "eric-odp-factory.securityPolicyReference" -}}
{{- $policyreference := "default-restricted-security-policy" -}}
{{- if .Values.global -}}
    {{- if .Values.global.security -}}
        {{- if .Values.global.security.policyReferenceMap -}}
            {{- if hasKey .Values.global.security.policyReferenceMap "default-restricted-security-policy" -}}
                {{- $policyreference = index .Values "global" "security" "policyReferenceMap" "default-restricted-security-policy" -}}
            {{- end -}}
        {{- end -}}
    {{- end -}}
{{- end -}}
{{- $policyreference -}}
{{- end -}}


{{/*
To support Dual stack.
*/}}
{{- define "eric-odp-factory.internalIPFamily" -}}
{{- if  .Values.global -}}
  {{- if  .Values.global.internalIPFamily -}}
    {{- .Values.global.internalIPFamily | toString -}}
  {{- else -}}
    {{- "none" -}}
  {{- end -}}
{{- else -}}
{{- "none" -}}
{{- end -}}
{{- end -}}


{{/*
DR-D470222-010
Configuration of Log Collection Streaming Method
*/}}
{{- define "eric-odp-factory.log-streamingMethod" -}}
{{- $defaultMethod := "indirect" }}
{{- if .Values.global -}}
    {{- if .Values.global.log -}}
        {{- if .Values.global.log.streamingMethod -}}
            {{- $streamingMethod := .Values.global.log.streamingMethod }}
                {{- if not $streamingMethod }}
                    {{- if (.Values.global.log).streamingMethod -}}
                        {{- $streamingMethod = (.Values.global.log).streamingMethod }}
                    {{- else -}}
                        {{- $streamingMethod = $defaultMethod -}}
                    {{- end }}
                {{- end }}

                {{- if or (eq $streamingMethod "direct") (eq $streamingMethod "indirect") }}
                    {{- $streamingMethod -}}
                {{- else }}
                    {{- $defaultMethod -}}
                {{- end }}
            {{- end }}
        {{- end }}
    {{- end }}
{{- end }}

{{- define "eric-odp-factory.log-streaming-activated" }}
  {{- $streamingMethod := (include "eric-odp-factory.log-streamingMethod" .) -}}
  {{- if or (eq $streamingMethod "dual") (eq $streamingMethod "direct") -}}
    {{- printf "%t" true -}}
  {{- else -}}
    {{- printf "%t" false -}}
  {{- end -}}
{{- end -}}

{{/*
DR-D1126-005 Set resources for the container
*/}}
{{- define "eric-odp-factory.containerResources" -}}
{{- $container := index . -}}
requests:
{{- if $container.requests.cpu }}
  cpu: {{ $container.requests.cpu | quote }}
{{- end }}
{{- if $container.requests.memory }}
  memory: {{ $container.requests.memory | quote }}
{{- end }}
{{- if (index $container "requests" "ephemeral-storage") }}
  ephemeral-storage: {{ (index $container "requests" "ephemeral-storage" | quote) }}
{{- end }}
limits:
{{- if $container.limits.cpu }}
  cpu: {{ $container.limits.cpu | quote }}
{{- end }}
{{- if $container.limits.memory }}
  memory: {{ $container.limits.memory | quote }}
{{- end }}
{{- if (index $container "limits" "ephemeral-storage") }}
  ephemeral-storage: {{ (index $container "limits" "ephemeral-storage" | quote) }}
{{- end }}
{{- end -}}

{{/*
Define the appArmor annotation creation based on input argument DR-D1123-127
*/}}
{{- define "eric-odp-factory.appArmorAnnotation.getAnnotation" -}}
{{- $profile := index . "profile" -}}
{{- $containerName := index . "containerName" -}}
{{- if $profile.type -}}
{{- if eq "runtime/default" (lower $profile.type) }}
container.apparmor.security.beta.kubernetes.io/{{ $containerName }}: "runtime/default"
{{- else if eq "unconfined" (lower $profile.type) }}
container.apparmor.security.beta.kubernetes.io/{{ $containerName }}: "unconfined"
{{- else if eq "localhost" (lower $profile.type) }}
{{- if $profile.localhostProfile }}
{{- $localhostProfileList := (split "/" $profile.localhostProfile) -}}
{{- if $localhostProfileList._1 }}
container.apparmor.security.beta.kubernetes.io/{{ $containerName }}: "localhost/{{ $localhostProfileList._1 }}"
{{- end }}
{{- end }}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Define the appArmor annotation for the factory container
*/}}
{{- define "eric-odp-factory.appArmorAnnotation.factory" -}}
{{- if .Values.appArmorProfile -}}
{{- $profile := .Values.appArmorProfile }}
{{- if .Values.appArmorProfile.factory -}}
{{- $profile = .Values.appArmorProfile.factory }}
{{- end -}}
{{- include "eric-odp-factory.appArmorAnnotation.getAnnotation" (dict "profile" $profile "containerName" "eric-odp-factory") }}
{{- end -}}
{{- end -}}


{{/*
Define the appArmor annotations
*/}}
{{- define "eric-odp-factory.appArmorAnnotations" -}}
{{- include "eric-odp-factory.appArmorAnnotation.factory" .}}
{{- if eq "true" (include "eric-odp-factory.log-streaming-activated" .) }}
{{- include "eric-log-shipper-sidecar.LsAppArmorProfileAnnotation" .}}
{{- end }}
{{- end -}}