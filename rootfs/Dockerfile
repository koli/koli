FROM quay.io/koli/base:0.1.0

RUN adduser --system \
    --shell /bin/bash \
    --disabled-password \
    --no-create-home \
    --group koli

USER koli
COPY . /

# TODO: Write a script to change between gitserver and gitreceiver
ENTRYPOINT ["/usr/bin/koli-controller"]
