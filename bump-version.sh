#!/usr/bin/env bash
set -e

# bump-version.sh: Automatically bumps SemVer version, updates Go source, commits and tags it.

# 1. Determine current version
LATEST_TAG=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.1.0")
CURRENT_VERSION=${LATEST_TAG#v}

echo "Current version: v$CURRENT_VERSION"

# 2. Parse bump type
BUMP_TYPE=${1:-patch}

# Split version parts
IFS='.' read -r major minor patch <<< "$CURRENT_VERSION"

case "$BUMP_TYPE" in
  major)
    major=$((major + 1))
    minor=0
    patch=0
    ;;
  minor)
    minor=$((minor + 1))
    patch=0
    ;;
  patch)
    patch=$((patch + 1))
    ;;
  *)
    echo "Usage: $0 [major|minor|patch]"
    exit 1
    ;;
esac

NEW_VERSION="$major.$minor.$patch"
NEW_TAG="v$NEW_VERSION"

echo "Bumping to: $NEW_TAG"

# 3. Update version string in internal/discord/bot.go
BOT_GO_PATH="internal/discord/bot.go"

if [ -f "$BOT_GO_PATH" ]; then
  # Use sed to update Version = "X.Y.Z" while preserving existing whitespace
  if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS sed uses \1 for backreferences
    sed -i '' -E "s/(Version[[:space:]]*)=[[:space:]]*\"[0-9]+\.[0-9]+\.[0-9]+\"/\1= \"$NEW_VERSION\"/g" "$BOT_GO_PATH"
  else
    # GNU sed
    sed -i -E "s/(Version[[:space:]]*)=[[:space:]]*\"[0-9]+\.[0-9]+\.[0-9]+\"/\1= \"$NEW_VERSION\"/g" "$BOT_GO_PATH"
  fi
  echo "Updated version string in $BOT_GO_PATH (preserved alignment)"
else
  echo "Warning: $BOT_GO_PATH not found, skipping source version update."
fi

# 4. Git commit & tag
git add "$BOT_GO_PATH"
git commit -m "Bump version to $NEW_VERSION"
git tag -a "$NEW_TAG" -m "Release $NEW_TAG"

echo "=============================================="
echo "Successfully bumped version to $NEW_TAG!"
echo "Run the following command to push and trigger the CI Release:"
echo "  git push && git push --tags"
echo "=============================================="
