{{- define "ingress_nginx_resources_requests_as_resources_management" -}}
{{- $resourcesRequests := . | default dict -}}
{{- $mode := $resourcesRequests.mode | default "VPA" -}}
{{- $resourcesManagement := dict "mode" $mode -}}

{{- if eq $mode "Static" -}}
  {{- $staticSource := $resourcesRequests.static | default dict -}}
  {{- $static := dict "requests" (dict
        "cpu" ($staticSource.cpu | default "350m")
        "memory" ($staticSource.memory | default "500Mi")) -}}
  {{- $limitsSource := $staticSource.limits | default dict -}}
  {{- $limits := dict -}}
  {{- if hasKey $limitsSource "cpu" -}}
    {{- $_ := set $limits "cpu" $limitsSource.cpu -}}
  {{- end -}}
  {{- if hasKey $limitsSource "memory" -}}
    {{- $_ := set $limits "memory" $limitsSource.memory -}}
  {{- end -}}
  {{- if gt (len $limits) 0 -}}
    {{- $_ := set $static "limits" $limits -}}
  {{- end -}}
  {{- $_ := set $resourcesManagement "static" $static -}}
{{- else -}}
  {{- $vpaSource := $resourcesRequests.vpa | default dict -}}
  {{- $cpuSource := $vpaSource.cpu | default dict -}}
  {{- $memorySource := $vpaSource.memory | default dict -}}
  {{- $cpu := dict
        "min" ($cpuSource.min | default "10m")
        "max" ($cpuSource.max | default "50m") -}}
  {{- $memory := dict
        "min" ($memorySource.min | default "50Mi")
        "max" ($memorySource.max | default "200Mi") -}}
  {{- if hasKey $cpuSource "limitRatio" -}}
    {{- $_ := set $cpu "limitRatio" $cpuSource.limitRatio -}}
  {{- end -}}
  {{- if hasKey $memorySource "limitRatio" -}}
    {{- $_ := set $memory "limitRatio" $memorySource.limitRatio -}}
  {{- end -}}
  {{- $_ := set $resourcesManagement "vpa" (dict
        "mode" ($vpaSource.mode | default "Initial")
        "cpu" $cpu
        "memory" $memory) -}}
{{- end -}}

{{- $resourcesManagement | toYaml -}}
{{- end -}}
