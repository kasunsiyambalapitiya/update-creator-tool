#!/bin/bash

target="wum-uc.go"
version="3.1.0"

type glide >/dev/null 2>&1 || { echo >&2 "Glide dependency management is needed to build the Update Creator Tool (https://glide.sh/).  Aborting."; exit 1; }

echo "Setting up dependencies..."
glide install

platforms="darwin/amd64/macosx/x64 linux/386/linux/i586 linux/amd64/linux/x64 windows/386/windows/i586 windows/amd64/windows/x64"

for platform in ${platforms}
do
    split=(${platform//\// })
    goos=${split[0]}
    goarch=${split[1]}
    pos=${split[2]}
    parch=${split[3]}

    echo "Building wum-uc for ${goos}/${goarch} platform..."

    # ensure output file name
    output="${binary}"
    test "${output}" || output="$(basename ${target} | sed 's/\.go//')"

    # add exe to windows output
    [[ "windows" == "${goos}" ]] && output="${output}.exe"

    zipfile="wum-uc-${version}-${pos}-${parch}"
    zipdir="$(dirname ${target})/build/target/${zipfile}"
    mkdir -p ${zipdir}

    cp -r "$(dirname ${target})/README.md" ${zipdir}
    cp -r "$(dirname ${target})/LICENSE.txt" ${zipdir}

    # set destination path for binary
    destination="${zipdir}/bin/${output}"

    #echo "GOOS=$goos GOARCH=$goarch go build -x -o $destination $target"
    GOOS=${goos} GOARCH=${goarch} go build -gcflags=-trimpath=$GOPATH -asmflags=-trimpath=$GOPATH -ldflags "-X main.version=${version} -X 'main.buildDate=$(date -u '+%Y-%m-%d %H:%M:%S')'" -o ${destination} ${target}

    pwd=`pwd`
    cd "$(dirname ${target})/build/target"
    zip -r "${zipfile}.zip" ${zipfile} > /dev/null 2>&1
    rm -rf ${zipfile}
    cd ${pwd}
done
