#! /bin/bash

set -e
set -x
SILENT="y"

silence() {
  if [ -n "$SILENT" ]; then
    "$@" >/dev/null
  else
    "$@"
  fi
}

OPENSSL_VERSION="1.1.1s"
ZLIB_VERSION="1.2.13"
PROTOBUF_VERSION="3.11.4"
CURL_VERSION="7.86.0"
AWS_SDK_CPP_VERSION="1.10.56"

LIB_OPENSSL="https://www.openssl.org/source/openssl-${OPENSSL_VERSION}.tar.gz"
LIB_ZLIB="https://zlib.net/fossils/zlib-${ZLIB_VERSION}.tar.gz"
LIB_PROTOBUF="https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOBUF_VERSION}/protobuf-all-${PROTOBUF_VERSION}.tar.gz"
LIB_CURL="https://curl.haxx.se/download/curl-${CURL_VERSION}.tar.gz"
CA_CERT="https://curl.se/ca/cacert.pem"

INSTALL_DIR=$(pwd)/third_party

#Figure out the release type from os. The release type will be used to determine the final storage location
# of the native binary
function find_release_type() {
  if [[ $OSTYPE == "linux-gnu" ]]; then
    echo "linux-$(uname -m)"
    return
  elif [[ $OSTYPE == darwin* ]]; then
    echo "osx"
    return
  elif [[ $OSTYPE == "msys" ]]; then
    echo "windows"
    return
  fi

  echo "unknown"
}

CMAKE=$(which cmake3 &>/dev/null && echo "cmake3 " || echo "cmake")
RELEASE_TYPE=$(find_release_type)

[[ $RELEASE_TYPE == "unknown" ]] && {
  echo "Could not define release type for $OSTYPE"
  exit 1
}

if [ "$1" == "clang" ] || [ "$(uname)" == 'Darwin' ]; then
  export MACOSX_DEPLOYMENT_TARGET='10.13'
  export MACOSX_MIN_COMPILER_OPT="-mmacosx-version-min=${MACOSX_DEPLOYMENT_TARGET}"
  export CC=$(which clang)
  export CXX=$(which clang++)
  export CXXFLAGS="-I$INSTALL_DIR/include -O3 -stdlib=libc++ ${MACOSX_MIN_COMPILER_OPT} "
  export CFLAGS="${MACOSX_MIN_COMPILER_OPT} "
  export C_INCLUDE_PATH="$INSTALL_DIR/include"

  if [ "$(uname)" == 'Linux' ]; then
    export LDFLAGS="-L$INSTALL_DIR/lib -nodefaultlibs -lpthread -ldl -lc++ -lc++abi -lm -lc -lgcc_s"
    export LD_LIBRARY_PATH="$INSTALL_DIR/lib:$LD_LIBRARY_PATH"
  else
    export LDFLAGS="-L$INSTALL_DIR/lib"
    export DYLD_LIBRARY_PATH="$INSTALL_DIR/lib:$DYLD_LIBRARY_PATH"
  fi
else
  export CC="gcc"
  export CXX="g++"
  export CXXFLAGS="-I$INSTALL_DIR/include -O3  -Wno-implicit-fallthrough -Wno-int-in-bool-context"
  export LDFLAGS="-L$INSTALL_DIR/lib "
  export LD_LIBRARY_PATH="$INSTALL_DIR/lib:$LD_LIBRARY_PATH"
fi

SED="sed -i"
if [[ "$OSTYPE" == "darwin"* ]]; then
  SED="sed -i ''"
fi

# Need to unset LD_LIBRARY_PATH for curl because the OpenSSL we build doesn't
# have MD4, which curl tries to use.
function _curl {
  #(unset LD_LIBRARY_PATH; curl -L $@)
  curl -L --cacert "$INSTALL_DIR/cacert.pem" $@
}

cd $INSTALL_DIR
wget --no-check-certificate -P $INSTALL_DIR $CA_CERT

function conf {
  if [[ "$OSTYPE" == "darwin"* ]]; then
    silence ./configure \
      --prefix="$INSTALL_DIR" \
      DYLD_LIBRARY_PATH="$DYLD_LIBRARY_PATH" \
      LDFLAGS="$LDFLAGS" \
      CXXFLAGS="$CXXFLAGS" \
      $@
  else
    silence ./configure \
      --prefix="$INSTALL_DIR" \
      LD_LIBRARY_PATH="$LD_LIBRARY_PATH" \
      LDFLAGS="$LDFLAGS" \
      CXXFLAGS="$CXXFLAGS" \
      C_INCLUDE_PATH="$C_INCLUDE_PATH" \
      $@
  fi
}

# Google Protocol Buffers
if [ ! -d "protobuf-${PROTOBUF_VERSION}" ]; then
  _curl "$LIB_PROTOBUF" >protobuf.tgz
  tar xf protobuf.tgz
  rm protobuf.tgz

  cd protobuf-${PROTOBUF_VERSION}
  silence conf --enable-shared=no
  silence make -j 4
  silence make install

  cd ..
fi

# AWS C++ SDK
if [ ! -d "aws-sdk-cpp" ]; then
  git clone https://github.com/awslabs/aws-sdk-cpp.git aws-sdk-cpp
  pushd aws-sdk-cpp
  git checkout ${AWS_SDK_CPP_VERSION}
  git submodule update --init --recursive
  popd

  rm -rf aws-sdk-cpp-build
  mkdir aws-sdk-cpp-build

  cd aws-sdk-cpp-build

  silence $CMAKE \
    -DBUILD_ONLY="core;kinesis;monitoring;sts" \
    -DCMAKE_BUILD_TYPE=RelWithDebInfo \
    -DSTATIC_LINKING=1 \
    -DCMAKE_PREFIX_PATH="$INSTALL_DIR" \
    -DCMAKE_C_COMPILER="$CC" \
    -DCMAKE_CXX_COMPILER="$CXX" \
    -DCMAKE_CXX_FLAGS="$CXXFLAGS" \
    -DCMAKE_INSTALL_PREFIX="$INSTALL_DIR" \
    -DCMAKE_FIND_FRAMEWORK=LAST \
    -DENABLE_TESTING="OFF" \
    ../aws-sdk-cpp
  silence make -j8
  silence make install

  cd ..

fi

cd ..

set +e
set +x
