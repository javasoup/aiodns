#!/bin/bash

name='aiodns'

MD5='md5sum'
if [[ "$(uname)" == 'Darwin' ]]; then
	MD5='md5'
fi

# UPX=false
# if hash upx 2>/dev/null; then
# 	UPX=true
# fi

VERSION=$(curl -sSL https://api.github.com/repos/javasoup/aiodns/commits/master | sed -n '{/sha/p; /date/p;}'| sed 's/.* \"//g' | cut -c1-10 | tr '[:lower:]' '[:upper:]' | sed 'N;s/\n/@/g' | head -1)
LDFLAGS="-X main.version=$VERSION -s -w"

# X86-64
OSES=(windows linux darwin freebsd)
ARCHS=(amd64)
rm -rf ./release
mkdir -p ./release
for os in ${OSES[@]}; do
	for arch in ${ARCHS[@]}; do
		suffix=""
		if [ "$os" == "windows" ]; then
			suffix=".exe"
			LDFLAGS="-X main.version=$VERSION -s -w"
		fi
		env CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -ldflags "$LDFLAGS" -o ./release/${name}_${os}_${arch}${suffix} .
		# if $UPX; then upx -9 ./release/${name}_${os}_${arch}${suffix}; fi
		tar -C ./release -zcf ./release/${name}_${os}-${arch}-$VERSION.tar.gz ./${name}_${os}_${arch}${suffix}
		$MD5 ./release/${name}_${os}-${arch}-$VERSION.tar.gz
	done
done

# ARM
ARMS=(5 6 7)
for v in ${ARMS[@]}; do
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=$v go build -ldflags "$LDFLAGS" -o ./release/${name}_arm$v .
done
# if $UPX; then upx -9 ./release/${name}_arm*; fi
tar -C ./release -zcf ./release/${name}_arm-$VERSION.tar.gz $(for v in ${ARMS[@]}; do echo -n "./${name}_arm$v "; done)
$MD5 ./release/${name}_arm-$VERSION.tar.gz

# MIPS # go 1.8+ required
LDFLAGS="-X main.version=$VERSION -s -w"
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle go build -ldflags "$LDFLAGS" -o ./release/${name}_mipsle .
env CGO_ENABLED=0 GOOS=linux GOARCH=mips go build -ldflags "$LDFLAGS" -o ./release/${name}_mips .
# MIPS # go 1.10+ required
env CGO_ENABLED=0 GOOS=linux GOARCH=mipsle GOMIPS=softfloat go build -ldflags "$LDFLAGS" -o ./release/${name}_mipsle_sf .
env CGO_ENABLED=0 GOOS=linux GOARCH=mips GOMIPS=softfloat go build -ldflags "$LDFLAGS" -o ./release/${name}_mips_sf .

# if $UPX; then upx -9 ./release/${name}_mips**; fi
tar -C ./release -zcf ./release/${name}_mipsle-$VERSION.tar.gz ./${name}_mipsle
tar -C ./release -zcf ./release/${name}_mips-$VERSION.tar.gz ./${name}_mips
tar -C ./release -zcf ./release/${name}_mipsle-sf-$VERSION.tar.gz ./${name}_mipsle_sf
tar -C ./release -zcf ./release/${name}_mips-sf-$VERSION.tar.gz ./${name}_mips_sf
$MD5 ./release/${name}_mipsle-$VERSION.tar.gz
