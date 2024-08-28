
ARG CBOS_IMAGE_NAME
ARG CBOS_IMAGE_REPO
ARG CBOS_IMAGE_TAG

FROM ${CBOS_IMAGE_REPO}/${CBOS_IMAGE_NAME}:${CBOS_IMAGE_TAG}

COPY ./build/go-binary/eric-odp-factory /usr/bin/eric-odp-factory

ARG ERIC_ODP_FACTORY_UID=141612
ARG ERIC_ODP_FACTORY_GID=141612

ARG GIT_COMMIT=""

RUN echo "${ERIC_ODP_FACTORY_UID}:x:${ERIC_ODP_FACTORY_UID}:${ERIC_ODP_FACTORY_GID}:eric-odp-factory-user:/:/bin/bash" >> /etc/passwd && \
    cat /etc/passwd && \
    sed -i "s|root:/bin/bash|root:/bin/false|g" /etc/passwd && \
    chmod -R g=u /usr/bin/eric-odp-factory && \
    chown -h ${ERIC_ODP_FACTORY_UID}:0 /usr/bin/eric-odp-factory

USER $ERIC_ODP_FACTORY_UID:$ERIC_ODP_FACTORY_GID

CMD ["/usr/bin/eric-odp-factory"]
