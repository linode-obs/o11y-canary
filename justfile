# Justfile

# Step to create and push the next git tag
release-tag:
    git tag -a $(svu next) -m "$(svu next) release"
    git push origin $(svu next)

# Step to run Goreleaser
goreleaser:
    sudo -E goreleaser release --clean

# Main command to execute all steps
deploy: release-tag goreleaser
    @echo "Deployment complete."
