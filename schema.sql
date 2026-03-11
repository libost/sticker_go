CREATE TABLE IF NOT EXISTS USERPOOL (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at INTEGER DEFAULT (unixepoch()),
    user_id INTEGER NOT NULL UNIQUE,
    obfu_id TEXT NOT NULL UNIQUE,
    last_cycle_starts_at INTEGER DEFAULT (unixepoch()),
    usage_count INTEGER DEFAULT 0,
    total_usage_count INTEGER DEFAULT 0,
    user_group TEXT DEFAULT 'user',
    timezone TEXT
); 

CREATE TABLE IF NOT EXISTS GRACE_KEY (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    generated_at INTEGER DEFAULT (unixepoch()),
    operator INTEGER NOT NULL,
    expires_at INTEGER DEFAULT (unixepoch()+3600),
    uuid TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS DONATION_LOG (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timecurrent INTEGER DEFAULT (unixepoch()),
    user_id INTEGER NOT NULL,
    amount REAL NOT NULL,
    payload TEXT NOT NULL UNIQUE,
    telegram_payment_charge_id TEXT NOT NULL,
    provider_payment_charge_id TEXT NOT NULL,
    status TEXT DEFAULT 'pending'
);