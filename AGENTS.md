See PROJECT.md for project description.

## Stack
Ruby on Rails 8.1, PostgreSQL (pgvector/pgvector), rspec.
UI: React, Tailwind CSS, monorepo

## Key commands
- `bin/dev` — run
- `RAILS_ENV=test RACK_ENV=test bundle exec rspec spec` — tests
- `bundle exec rails db:migrate` — migrate (если применимо)
- TODO: Add how run UI stack

## Conventions
- Rails MVC with commands and form object pattern
- RSpec for tests, FactoryBot for fixtures
- Use gems instead of writing your own implementation.
- Solve the problem, not the consequence 

## Constraints
- Don't touch existing migrations
- Consult with me when choosing a library