-- Migration 002: Make phone optional (email-only auth)
-- Run this after 001_init_mysql.sql

ALTER TABLE users
  MODIFY COLUMN phone VARCHAR(30) NULL DEFAULT NULL;

-- Drop the unique constraint on phone (allow multiple NULLs)
ALTER TABLE users
  DROP INDEX uq_users_phone;

-- Re-add as a partial unique index (only unique when not null)
-- MySQL handles NULL uniqueness correctly with standard UNIQUE KEY
ALTER TABLE users
  ADD UNIQUE KEY uq_users_phone (phone);

-- Add phone_auth_enabled to platform_settings if not exists
INSERT IGNORE INTO platform_settings (id, `key`, value, value_type, description)
VALUES (UUID(), 'phone_auth_enabled', 'false', 'boolean', 'Enable phone number + OTP registration and login');