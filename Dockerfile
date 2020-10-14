FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -y && apt-get upgrade -y && apt-get install -y vim wget curl golang
RUN apt-get update -y && apt-get upgrade -y && apt-get install -y git
RUN apt-get update -y && apt-get upgrade -y && apt-get install -y net-tools 

WORKDIR /root/go/src
RUN git clone https://github.com/sedillo/rtop
WORKDIR /root/go/src/rtop
RUN go get ./...
RUN go install rtop
ENV PATH=$PATH:/root/go/bin
RUN apt-get update -y && apt-get upgrade -y && apt-get install -y iproute2
