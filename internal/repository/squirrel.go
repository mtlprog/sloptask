package repository

import sq "github.com/Masterminds/squirrel"

// psql is the shared Squirrel statement builder configured for PostgreSQL dollar placeholders.
var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)
