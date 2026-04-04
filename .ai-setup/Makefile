# AI agents, CLI tools, skills, and plugins

BLUE := \033[0;34m
NC := \033[0m
PATH := $(HOME)/.local/bin:/usr/local/bin:/opt/homebrew/bin:$(PATH)
VERSION ?= 0.2.0

NPM ?= mise exec -- npm
CLAUDE ?= mise exec -- claude
SKILLS_NPX ?= mise exec -- npx
SKILLS ?= $(SKILLS_NPX) skills
AGENTS_TARGETS := codex claude-code
AGENTS_SKILLS_AGENT_FLAGS := $(foreach agent,$(AGENTS_TARGETS),-a $(agent))
CLAUDE_MARKETPLACE_NAMES ?= dapi
CLAUDE_PLUGIN_NAMESPACE ?= dapi

REGISTRY_REPO ?= thinknetica/ai_swe_group_1

GOOGLE_WORKSPACE_SKILLS := \
	gws-docs \
	gws-docs-write \
	gws-drive \
	gws-sheets

.PHONY: ai bootstrap mise-package mise-install version check extra extra-check check-context codex-context register
.PHONY: agents-install agents agents-cli agents-skills extra-skills
.PHONY: agents-skills-install agents-skills-list agents-skills-check-npx

ai: bootstrap
	@$(MAKE) agents-install

bootstrap: mise-package mise-install

version:
	@version="$(VERSION)"; \
	if printf '%s' "$$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?([+][0-9A-Za-z.-]+)?$$'; then \
		printf '%s\n' "$$version"; \
	else \
		echo "❌ VERSION must contain a semver value" >&2; \
		exit 1; \
	fi

check:
	@VERSION=$(VERSION) ./scripts/test-setup.sh

extra-check:
	@CLAUDE_MARKETPLACE_NAMES="$(CLAUDE_MARKETPLACE_NAMES)" \
	CLAUDE_EXPECTED_PLUGINS="$(CLAUDE_PLUGINS)" \
	./scripts/test-extras.sh

check-context:
	@./scripts/test-context.sh

codex-context:
	@./scripts/codex-context.sh

mise-package:
	@set -e; \
	if command -v mise > /dev/null 2>&1; then \
		echo "Install mise - already exists"; \
	elif command -v brew > /dev/null 2>&1; then \
		brew install mise; \
		echo "Install mise - installed via brew"; \
	elif command -v curl > /dev/null 2>&1; then \
		if [ -w /usr/local/bin ]; then \
			MISE_INSTALL_PATH=/usr/local/bin/mise; \
			SUDO=""; \
		elif command -v sudo > /dev/null 2>&1; then \
			MISE_INSTALL_PATH=/usr/local/bin/mise; \
			SUDO="sudo"; \
		else \
			mkdir -p "$(HOME)/.local/bin"; \
			MISE_INSTALL_PATH="$(HOME)/.local/bin/mise"; \
			SUDO=""; \
		fi; \
		curl -fsSL https://mise.run | $$SUDO env MISE_INSTALL_PATH="$$MISE_INSTALL_PATH" sh; \
		echo "Install mise - installed via mise.run to $$MISE_INSTALL_PATH"; \
	else \
		echo "Install mise - failed: need either brew or curl to install mise"; \
		exit 1; \
	fi

mise-install: mise-package
	@echo "Install mise tools"
	@mise install --quiet

agents-install: agents agents-cli agents-skills

# --- AI Agents ---

agents:
	@$(NPM) install -g @anthropic-ai/claude-code
	@$(NPM) install -g @openai/codex

# --- CLI tools used by agents ---

agents-cli:
	@$(NPM) install -g @playwright/cli@latest
	@if command -v ccbox >/dev/null 2>&1; then \
		echo "Install ccbox - already exists"; \
	elif command -v brew >/dev/null 2>&1; then \
		brew tap diskd-ai/ccbox && brew install ccbox; \
		echo "Install ccbox - installed via brew"; \
	elif command -v curl >/dev/null 2>&1; then \
		curl -fsSL -H 'Cache-Control: no-cache' -o - https://raw.githubusercontent.com/diskd-ai/ccbox/main/scripts/install.sh | /bin/bash; \
		echo "Install ccbox - installed via script"; \
	else \
		echo "⚠️  ccbox: need either brew or curl to install"; \
	fi

# --- Skills for agents ---

agents-skills: agents-skills-install

agents-skills-check-npx:
	@$(SKILLS_NPX) --version >/dev/null 2>&1 || (echo "❌ npx not available via mise. Run 'make ai' after bootstrap installs Node.js." && exit 1)

agents-skills-install: agents-skills-check-npx
	@echo "$(BLUE)📦 Installing core skills...$(NC)"
	@for dir in "$$HOME/.claude/skills" "$$HOME/.codex/skills"; do \
		if [ -d "$$dir/prompt-engeneering" ]; then \
			echo "  🗑️  Removing old prompt-engeneering (typo) from $$dir"; \
			rm -rf "$$dir/prompt-engeneering"; \
		fi; \
	done
	@echo "  📥 Installing playwright-cli from microsoft/playwright-cli"
	@$(SKILLS) add microsoft/playwright-cli --skill playwright-cli -g $(AGENTS_SKILLS_AGENT_FLAGS) -y
	@echo "  📥 Installing prompt-engineering from CodeAlive-AI/prompt-engineering-skill"
	@$(SKILLS) add CodeAlive-AI/prompt-engineering-skill@prompt-engineering -g -y
	@echo "  📥 Installing ccbox from diskd-ai/ccbox"
	@$(SKILLS) add diskd-ai/ccbox --skill ccbox -g $(AGENTS_SKILLS_AGENT_FLAGS) -y
	@echo "  📥 Installing ccbox-insights from diskd-ai/ccbox"
	@$(SKILLS) add diskd-ai/ccbox --skill ccbox-insights -g $(AGENTS_SKILLS_AGENT_FLAGS) -y

agents-skills-list:
	@echo "$(BLUE)📋 Core skills:$(NC)"
	@printf "  playwright-cli (microsoft/playwright-cli)\n"
	@printf "  prompt-engineering (CodeAlive-AI/prompt-engineering-skill)\n"
	@printf "  ccbox (diskd-ai/ccbox)\n"
	@printf "  ccbox-insights (diskd-ai/ccbox)\n"

# --- Extra skills and plugins (not installed by default) ---

extra: extra-skills

extra-skills: agents-skills-check-npx
	@echo "$(BLUE)📦 Installing extra CLIs, skills, and plugins...$(NC)"
	@mise install "github:pimalaya/himalaya"
	@$(NPM) install -g @dapi/tgcli
	@$(NPM) install -g @googleworkspace/cli
	@echo "  📥 Installing tgcli from dapi/tgcli"
	@$(SKILLS) add dapi/tgcli --skill tgcli -g $(AGENTS_SKILLS_AGENT_FLAGS) -y
	@for skill in $(GOOGLE_WORKSPACE_SKILLS); do \
		echo "  📥 Installing $$skill from googleworkspace/cli"; \
		$(SKILLS) add googleworkspace/cli --skill "$$skill" -g $(AGENTS_SKILLS_AGENT_FLAGS) -y; \
	done
	@for mp in $(CLAUDE_PLUGINS_MARKETPLACES); do \
		echo "  Adding marketplace $$mp..."; \
		$(CLAUDE) plugins marketplace add $$mp; \
	done
	@for plugin in $(CLAUDE_PLUGINS); do \
		echo "  Installing plugin $$plugin..."; \
		$(CLAUDE) plugins install $$plugin; \
	done

CLAUDE_PLUGINS_MARKETPLACES ?= dapi/claude-code-marketplace

CLAUDE_PLUGINS ?= \
	himalaya@$(CLAUDE_PLUGIN_NAMESPACE) \
	pr-review-fix-loop@$(CLAUDE_PLUGIN_NAMESPACE) \
	spec-reviewer@$(CLAUDE_PLUGIN_NAMESPACE) \
	zellij-workflow@$(CLAUDE_PLUGIN_NAMESPACE)

# --- Registry ---

register:
	@REPO_URL=$$(git remote get-url origin | sed 's|git@github.com:|https://github.com/|;s|\.git$$||'); \
	TMPDIR=$$(mktemp -d); \
	BRANCH="register-$$(echo "$$REPO_URL" | sed 's|https://github.com/||;s|/|-|g;s|[^a-zA-Z0-9-]|-|g')"; \
	echo "📋 Registering $$REPO_URL in $(REGISTRY_REPO)..."; \
	gh repo fork "$(REGISTRY_REPO)" --clone --remote --default-branch-only 2>/dev/null || true; \
	FORK_REPO="$$(gh api user -q .login)/$$(basename $(REGISTRY_REPO))"; \
	git clone "https://github.com/$$FORK_REPO.git" "$$TMPDIR/registry" && \
	cd "$$TMPDIR/registry" && \
	git remote add upstream "https://github.com/$(REGISTRY_REPO).git" && \
	git fetch upstream main --quiet && \
	git checkout -b "$$BRANCH" upstream/main && \
	echo "$$REPO_URL" >> REGISTRY.txt && \
	git add REGISTRY.txt && \
	git commit -m "Register $$REPO_URL" && \
	git push -u origin "$$BRANCH" && \
	gh pr create \
		--repo "$(REGISTRY_REPO)" \
		--title "Register $$REPO_URL" \
		--body "Adding \`$$REPO_URL\` to the registry." \
		--base main; \
	rm -rf "$$TMPDIR"
