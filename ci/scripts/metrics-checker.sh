#!/bin/sh

METRICS_EXPOSURE_TUTORIAL_URL="https://confluence-oss.seli.wh.rnd.internal.ericsson.com/pages/viewpage.action?spaceKey=ESO&title=How+to+add+metrics+to+a+microservice";

checkValuesYAML(){
    SERVICE_NAME=$1
    echo -e "prometheus:\n  path: /metrics\n  role: service\n  interval: 15s" > .bob/var.compareToLine;
    grep -A3 "^prometheus:" ./charts/$SERVICE_NAME/values.yaml > .bob/var.scrapedLine || true;
    if cmp -s .bob/var.compareToLine .bob/var.scrapedLine; then
        echo "SUCCESS: values.yaml contains all the lines necessary for metrics exposure.";
        echo "true" >> .bob/var.metrics-exposed;
    else
        echo -e "FAILURE: This stage has failed as the lines needed for metric exposure are not correctly implemented inside values.yaml.\nPlease refer to the page provided:\n$METRICS_EXPOSURE_TUTORIAL_URL";
        echo -e "What is needed:"
        cat .bob/var.compareToLine
        echo -e "\nWhat was provided:"
        cat .bob/var.scrapedLine
        echo "false" >> .bob/var.metrics-exposed;
    fi
}

checkServiceYAML(){
    SERVICE_NAME=$1
    if grep -q "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}" ./charts/$SERVICE_NAME/templates/service.yaml; then
        echo "SUCCESS: service.yaml contains all the lines necessary for metrics exposure.";
        echo "true" >> .bob/var.metrics-exposed;
    else
        echo -e "FAILURE: This stage has failed as the lines needed for metric exposure are not correctly implemented inside service.yaml.\nPlease refer to the page provided:\n$METRICS_EXPOSURE_TUTORIAL_URL";
        echo -e "What is needed:"
        echo -e "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}"
        echo -e "Where [0-9] is to be replaced by the indent number. Line has to be provided under 'annotations'."
        echo "false" >> .bob/var.metrics-exposed;
    fi
}

checkDeploymentYAML(){
    SERVICE_NAME=$1
    if grep -q "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}" ./charts/$SERVICE_NAME/templates/deployment.yaml; then
        echo "SUCCESS: deployment.yaml contains all the lines necessary for metrics exposure.";
        echo "true" >> .bob/var.metrics-exposed;
    else
        echo -e "FAILURE: This stage has failed as the lines needed for metric exposure are not correctly implemented inside deployment.yaml.\nPlease refer to the page provided:\n$METRICS_EXPOSURE_TUTORIAL_URL";
        echo -e "What is needed:"
        echo -e "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}"
        echo -e "Where [0-9] is to be replaced by the indent number."
        echo "false" >> .bob/var.metrics-exposed;
    fi
}

checkConfigMapYAML(){
    SERVICE_NAME=$1
    if grep -q "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}" ./charts/$SERVICE_NAME/templates/configmap.yaml; then
        echo "SUCCESS: configmap.yaml contains all the lines necessary for metrics exposure.";
        echo "true" >> .bob/var.metrics-exposed;
    else
        echo -e "FAILURE: This stage has failed as the lines needed for metric exposure are not correctly implemented inside configmap.yaml.\nPlease refer to the page provided:\n$METRICS_EXPOSURE_TUTORIAL_URL";
        echo -e "What is needed:"
        echo -e "{{- include \"$SERVICE_NAME.annotations-with-prometheus\" . | nindent [0-9] }}"
        echo -e "Where [0-9] is to be replaced by the indent number."
        echo "false" >> .bob/var.metrics-exposed;
    fi
}

checkHelperTPL(){
    SERVICE_NAME=$1
    echo -e "{{/*\nCreate prometheus info\n*/}}" > .bob/var.compareToLine;
    echo -e "{{- define \"$SERVICE_NAME.prometheus\" }}" >> .bob/var.compareToLine;
    echo -e "prometheus.io/scrape-role: {{ .Values.prometheus.role | quote }}" >> .bob/var.compareToLine;
    echo -e "prometheus.io/scrape-interval: {{ .Values.prometheus.interval | quote }}" >> .bob/var.compareToLine;
    echo -e "prometheus.io/path: {{ .Values.prometheus.path | quote }}" >> .bob/var.compareToLine;
    echo -e "prometheus.io/port: {{ .Values.service.metrics.port | quote }}" >> .bob/var.compareToLine;
    echo -e "{{- end }}" >> .bob/var.compareToLine;
    grep -B6 -A2 "^prometheus.io/path:" ./charts/$SERVICE_NAME/templates/_helpers.tpl > .bob/var.scrapedLine || true;
    if cmp -s .bob/var.compareToLine .bob/var.scrapedLine; then
        echo "SUCCESS: helper.yaml contains all the lines necessary for metrics exposure.";
        echo "true" >> .bob/var.metrics-exposed;
    else
        echo -e "FAILURE: This stage has failed as the lines needed for metric exposure are not correctly implemented inside helper.tpl.\nPlease refer to the page provided:\n$METRICS_EXPOSURE_TUTORIAL_URL";
        echo -e "What is needed:"
        cat .bob/var.compareToLine
        echo -e "\nWhat was provided:"
        cat .bob/var.scrapedLine
        echo "false" >> .bob/var.metrics-exposed;
    fi
}

passOrFailCheck(){
    if grep -q "false" .bob/var.metrics-exposed; then
        echo "FAILURE: Please review console output to find the files which should be corrected.";
        exit 0;
    else
        echo "SUCCESS: All necessary lines for metrics exposure implemented correctly.";
    fi
}
