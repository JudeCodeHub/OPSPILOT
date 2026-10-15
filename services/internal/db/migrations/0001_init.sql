-- Initial schema for the OpsPilot demo ticketing app.

CREATE TABLE venues (
    id       UUID PRIMARY KEY,
    name     TEXT NOT NULL,
    city     TEXT NOT NULL,
    capacity INT  NOT NULL
);

CREATE TABLE events (
    id          UUID PRIMARY KEY,
    name        TEXT        NOT NULL,
    artist      TEXT        NOT NULL,
    venue_id    UUID        NOT NULL REFERENCES venues (id),
    starts_at   TIMESTAMPTZ NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    theme       TEXT        NOT NULL -- key for generated artwork: aurora, neon, sunset, ...
);

CREATE TABLE seats (
    event_id    UUID NOT NULL REFERENCES events (id),
    seat_id     TEXT NOT NULL,                     -- e.g. 'A12'
    section     TEXT NOT NULL,                     -- 'VIP' | 'Premium' | 'Standard'
    row_label   TEXT NOT NULL,
    number      INT  NOT NULL,
    price_cents INT  NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'LKR',
    status      TEXT NOT NULL DEFAULT 'available', -- 'available' | 'sold'
    PRIMARY KEY (event_id, seat_id),
    CONSTRAINT seats_status_check CHECK (status IN ('available', 'sold'))
);

CREATE TABLE bookings (
    id           UUID PRIMARY KEY,
    event_id     UUID        NOT NULL,
    seat_id      TEXT        NOT NULL,
    email        TEXT        NOT NULL,
    amount_cents INT         NOT NULL,
    currency     TEXT        NOT NULL,
    charge_id    TEXT        NOT NULL,
    status       TEXT        NOT NULL, -- 'confirmed'
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Prevents selling the same seat twice, even if two checkouts race.
    CONSTRAINT bookings_event_seat_unique UNIQUE (event_id, seat_id)
);

CREATE TABLE tickets (
    code       TEXT PRIMARY KEY,
    booking_id UUID        NOT NULL UNIQUE,
    event_id   UUID        NOT NULL,
    seat_id    TEXT        NOT NULL,
    email      TEXT        NOT NULL,
    issued_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX tickets_email_issued_at_idx ON tickets (email, issued_at DESC);
