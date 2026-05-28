# ssm-session-client root Makefile
# Delegates testing tasks to test/scripts/Makefile
# Provides convenient shortcuts for development and testing.

.PHONY: help build test acceptance acceptance-prepare acceptance-wait-ssm acceptance-run-local acceptance-run-ci acceptance-destroy acceptance-sso-interactive acceptance-backend-init

TEST_SCRIPTS_DIR := test/scripts

help: ## Show this help message
	@echo "ssm-session-client — AWS SSM Session Manager client"
	@echo ""
	@echo "Build targets:"
	@grep -E '^\s+make.*?##' Makefile | awk 'BEGIN {FS = "##"}; {printf "  \033[36m%-40s\033[0m %s\n", $$1, $$2}' | sed 's/make //g'
	@echo ""
	@echo "Common test targets:"
	@echo "  make acceptance                          # Full lifecycle: provision, test, destroy (AWS_REGION=...)"
	@echo "  make acceptance-run-local               # Run acceptance tests (requires infrastructure provisioned)"
	@echo ""
	@echo "Acceptance test targets (from test/scripts/Makefile):"
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) help 2>/dev/null || echo "  (Run 'make help' in test/scripts/ for full list)"

build: ## Build ssm-session-client binary
	@go build -o ssm-session-client ./main.go
	@echo "✓ Built ssm-session-client"

test: ## Run unit tests with race detector
	@go test ./... -race -timeout 30s

acceptance: ## Full acceptance test lifecycle: prepare infrastructure, run tests, destroy
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance AWS_REGION=$(AWS_REGION)

acceptance-prepare: ## Provision test infrastructure (requires AWS credentials)
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-prepare AWS_REGION=$(AWS_REGION)

acceptance-wait-ssm: ## Wait for SSM agent to come online on provisioned instances
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-wait-ssm

acceptance-run-local: ## Run acceptance tests (requires infrastructure to be provisioned)
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-run-local

acceptance-run-ci: ## Run acceptance tests in CI (identical to local; relies on OIDC credentials)
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-run-ci

acceptance-destroy: ## Tear down all test infrastructure
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-destroy

acceptance-sso-interactive: ## Run interactive SSO login test (requires a human to approve in a browser)
	@if [ -z "$(SSO_PROFILE)" ]; then \
		echo "ERROR: SSO_PROFILE is required. Usage: make acceptance-sso-interactive SSO_PROFILE=my-sso-profile" >&2; \
		exit 1; \
	fi
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-sso-interactive SSO_PROFILE=$(SSO_PROFILE) SSO_TEST_TIMEOUT=$(SSO_TEST_TIMEOUT)

acceptance-backend-init: ## Reconfigure and reconnect to the Terraform backend (state bucket)
	@$(MAKE) -C $(TEST_SCRIPTS_DIR) acceptance-backend-init AWS_REGION=$(AWS_REGION)

lint: ## Run Go linter
	@golangci-lint run ./...

fmt: ## Format code with gofmt
	@gofmt -w .
	@echo "✓ Code formatted"

cover: ## Run tests with coverage report
	@go test ./... -race -coverprofile=coverage.out -covermode=atomic
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report generated: coverage.html"

clean: ## Clean build artifacts and test files
	@rm -f ssm-session-client coverage.out coverage.html
	@echo "✓ Cleaned build artifacts"
