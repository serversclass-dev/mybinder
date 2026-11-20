-- Fix admin password hash to use $2a$ prefix (Go bcrypt compatible)
-- Password is: Admin@123456
UPDATE users
SET password_hash = '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj/9p3pTq8Ei',
    is_active     = 1,
    is_verified   = 1,
    updated_at    = NOW()
WHERE email = 'admin@souqiq.ly';