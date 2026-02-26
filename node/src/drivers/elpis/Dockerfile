FROM docker.arvancloud.ir/ubuntu:24.04
WORKDIR /app

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update -y && apt-get upgrade -y && \
    apt-get install -y curl dpkg-dev g++ gcc git ninja-build \
    software-properties-common wget zlib1g-dev && \
    apt-get install -y cmake && \
    apt-get install -y clang-18 liblld-18-dev libpolly-18-dev llvm-18-dev && \
    apt-get clean && \
    update-alternatives --install /usr/bin/clang clang /usr/bin/clang-18 100 && \
    update-alternatives --install /usr/bin/clang++ clang++ /usr/bin/clang++-18 100 && \
    update-alternatives --install /usr/bin/llvm-strip llvm-strip /usr/bin/llvm-strip-18 100

ENV CC=/usr/bin/clang-18
ENV CXX=/usr/bin/clang++-18

RUN curl -sSf https://raw.githubusercontent.com/WasmEdge/WasmEdge/master/utils/install.sh | bash -s -- -v 0.14.0

ENV PATH="/root/.wasmedge/bin:$PATH"
ENV LD_LIBRARY_PATH="/root/.wasmedge/lib:$LD_LIBRARY_PATH"
ENV LIBRARY_PATH="/root/.wasmedge/lib:$LIBRARY_PATH"
ENV C_INCLUDE_PATH="/root/.wasmedge/include:$C_INCLUDE_PATH"
ENV CPLUS_INCLUDE_PATH="/root/.wasmedge/include:$CPLUS_INCLUDE_PATH"
ENV WASMEDGE_LIB_DIR="/root/.wasmedge/lib"

RUN apt-get install -y gdb

COPY . .

RUN g++ -o test -g test.cpp main.cpp -lwasmedge

ENTRYPOINT ["bash", "run.sh"]
