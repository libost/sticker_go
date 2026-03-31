CREATE TABLE IF NOT EXISTS USERPOOL (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER DEFAULT (unixepoch()),
    user_id INTEGER NOT NULL UNIQUE,
    obfu_id TEXT NOT NULL UNIQUE,
    last_cycle_starts_at INTEGER DEFAULT (unixepoch()),
    usage_count INTEGER DEFAULT 0,
    total_usage_count INTEGER DEFAULT 0,
    user_group TEXT DEFAULT 'user',
    language_code TEXT,
    timezone TEXT
); 

CREATE TABLE IF NOT EXISTS GRACE_KEY (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    generated_at INTEGER DEFAULT (unixepoch()),
    operator INTEGER NOT NULL,
    expires_at INTEGER DEFAULT (unixepoch()+3600),
    uuid TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS DONATION_LOGS (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp INTEGER DEFAULT (unixepoch()),
    user_id INTEGER NOT NULL,
    amount INTEGER NOT NULL,
    payload TEXT NOT NULL UNIQUE,
    telegram_payment_charge_id TEXT NOT NULL,
    provider_payment_charge_id TEXT NOT NULL,
    status TEXT DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS STATISTICS (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    weekday TEXT NOT NULL UNIQUE,
    daily_usage_count INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS LAST_CLEANUP (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    last_cleanup_at INTEGER DEFAULT (unixepoch())
);