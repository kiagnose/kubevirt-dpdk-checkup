ARG BASE_IMAGE_TAG
FROM registry.access.redhat.com/ubi9/ubi-minimal:$BASE_IMAGE_TAG

RUN microdnf install -y shadow-utils && \
  adduser --system --no-create-home -u 900 dpdk-checkup && \
  microdnf remove -y shadow-utils && \
  microdnf clean all

COPY  _output/bin/kubevirt-dpdk-checkup /usr/local/bin

USER 900

ENTRYPOINT ["kubevirt-dpdk-checkup"]
