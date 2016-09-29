#!/bin/bash

# Handle the building and uploading of binaries to GitHub releases.

# There are 2 build processes:
# - one on any Linux machine (assume glibc), using GOOS.
# - the other is within an Alpine Linux container, so it builds against musl libc as an interpreter.

target_platforms="linux_amd64 linux_musl_amd64"

if [ $# -ne 1 ]; then
  echo "Usage: $0 version_number"
  exit 1
fi

which github-release >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "Install github-release first, run: go get github.com/aktau/github-release"
  exit 1
fi

which zip >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "zip utility not found, cannot proceed."
  exit 1
fi

which docker >/dev/null 2>&1
if [ $? -ne 0 ]; then
  echo "docker not found, cannot proceed."
  exit 1
fi

github_user=$(jq -e -r .user ~/.github_credentials.json)
github_token=$(jq -e -r .token ~/.github_credentials.json)
if [[ -z "$github_user" || "$github_user" == "null" ]]; then
  echo "Cannot find your GitHub username, do you have a ~/.github_credentials.json file? It should have a key called 'user'."
  exit 1
fi
if [[ -z "$github_token" || "$github_token" == "null" ]]; then
  echo "Cannot find your GitHub token, do you have a ~/.github_credentials.json file? It should have a key called 'token'."
  exit 1
fi

# TODOLATER: validate version# matches semver standards
new_version="$1"
echo "New Version: ${new_version}"
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

set -e

# Adjust version.go
echo "$(date) : Adjusting version.go to reflect new version..."
if [[ $OSTYPE =~ ^darwin ]]; then
  sed -i .bak "s/ecs_discoverer_version = \".*\"$/ecs_discoverer_version = \"${new_version}\"/" "${DIR}/version.go"
  rm -f "${DIR}/version.go.bak"
else
  sed -i "s/ecs_discoverer_version = \".*\"$/ecs_discoverer_version = \"${new_version}\"/" "${DIR}/version.go"
fi
if [ ! -z "$(git status -s | grep "version.go$")" ]; then
  git pull
  git commit -m "Bump version.go for new release ${new_version}" version.go
  git push
fi
echo "$(date) : Done."

# Remove any old binaries and zips first.
rm -f "${DIR}/bin/*"

# Build for various OSes/archs
echo "$(date) : Building for Linux (glibc)..."
GOOS=linux GOARCH=amd64 go build -o "${DIR}/bin/ecs-discoverer-${new_version}-linux_amd64"
echo "$(date) : Build for Linux completed"
#
echo "$(date) : Building for Alpine Linux (musl libc)..."
build_image="ecs_discoverer_build"
build_container="ecs_discoverer_builder"
docker build -t "$build_image" .
docker run -d --name "$build_container" "$build_image" tail -f /dev/null
echo "$(date) : Fetching golang dependencies within container"
docker exec -it "$build_container" sh -c "cd /tmp/ecs-discoverer && go get -d"
echo "$(date) : Running go build within container"
docker exec -it "$build_container" sh -c "cd /tmp/ecs-discoverer && GOOS=linux GOARCH=amd64 go build"
docker cp "$build_container:/tmp/ecs-discoverer/ecs-discoverer" "${DIR}/bin/ecs-discoverer-${new_version}-linux_musl_amd64"
trap "echo 'Cleaning up containers/images'; docker stop -t 1 $build_container && docker rm $build_container && docker rmi $build_image" EXIT QUIT TERM
echo "$(date) : Build for Alpine Linux completed"

# Create git tag
echo "$(date) : Tagging and pushing"
git tag "${new_version}"
git push --tags
echo "$(date) : Tagged and pushed"

echo "$(date) : Generate SHA256 hashes of new binaries..."
binary_sha256s="Notes go here.
\`\`\`
"
for i in $target_platforms; do
  binary_suffix=${i/-/_}
  echo "$binary_suffix"
  binary_sha256s="${binary_sha256s}ecs-discoverer-${new_version}-${binary_suffix}
  $(shasum -a 256 "${DIR}/bin/ecs-discoverer-${new_version}-${binary_suffix}" | awk '{print $1}')
  
"
done
binary_sha256s="${binary_sha256s}\`\`\`"
echo "$(date) : Hashes generated."

# Create GitHub release
echo "$(date) : Creating release"
github-release release --security-token "$github_token" --user CpuID --repo ecs-discoverer --tag "${new_version}" --name "ecs-discoverer ${new_version}" --description "${binary_sha256s}" --pre-release
echo "$(date) : Release created"

# Zip up binaries
echo "$(date) : Zip up binaries for upload to GitHub"
cd "${DIR}/bin"
for i in $target_platforms; do
  binary_suffix=${i/-/_}
  echo "$binary_suffix"
  zip "ecs-discoverer-${new_version}-${binary_suffix}.zip" "ecs-discoverer-${new_version}-${binary_suffix}"
done
cd "$DIR"
echo "$(date) : Zip files of binaries created"

# Push binaries up to GitHub
echo "$(date) : Pushing binaries (zipped) to GitHub release"
for i in $target_platforms; do
  binary_suffix=${i/-/_}
  echo "$binary_suffix"
  # Was getting random 502's on these uploads in the past, add a sleep to see if it helps.
  sleep 1
  github-release upload --security-token "$github_token" --user CpuID --repo ecs-discoverer --tag "${new_version}" --name "ecs-discoverer-${new_version}-${binary_suffix}.zip" --file "${DIR}/bin/ecs-discoverer-${new_version}-${binary_suffix}.zip"
done
echo "$(date) : Binaries pushed to GitHub"
