FROM quay.io/koli/base:v0.2.0

RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys E1DF1F24 \
        && echo "deb http://ppa.launchpad.net/git-core/ppa/ubuntu xenial main" >> /etc/apt/sources.list \
        && apt-get update \
        && apt-get install -y \
                make \
                tar \
                upx-ucl \
                git \
                --no-install-recommends \
        && apt-get clean \
        && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/* /usr/share/man /usr/share/doc 

ENV SHORTNAME koli
RUN mkdir -p /go/src/kolihub.io/${SHORTNAME}
WORKDIR /go/src/kolihub.io/${SHORTNAME}
ADD . /go/src/kolihub.io/${SHORTNAME}        

# DOWNLOAD GO
RUN curl --progress-bar -L https://storage.googleapis.com/golang/go1.7.5.linux-amd64.tar.gz | tar -xz -C /usr/local/ 
ENV GOPATH /go
ENV GOROOT /usr/local/go
ENV PATH $GOPATH/bin:$GOROOT/bin:$PATH

RUN curl --progress-bar -L https://s3.amazonaws.com/koli-vendors/vendor-koli.tar.gz | tar -xz -C /go/src/kolihub.io/koli/

RUN make build-local

RUN adduser --system \
    --shell /bin/bash \
    --disabled-password \
    --no-create-home \
    --group koli

RUN cp -a /go/src/kolihub.io/koli/rootfs/* /

# Clean Image
RUN rm -Rf /usr/local/go /go
RUN apt-get remove --purge xz-utils upx-ucl tzdata perl make makedev git -y
RUN apt-get autoremove -y

WORKDIR /

USER koli
ENTRYPOINT ["/usr/bin/koli-controller"]