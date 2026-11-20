DELETE FROM users WHERE email = 'admin@souqiq.ly';

INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified, created_at, updated_at)
VALUES (
  'admin-0000-0000-0000-000000000001',
  NULL,
  'admin@souqiq.ly',
  '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj/9p3pTq8Ei',
  'super_admin',
  1,
  1,
  NOW(),
  NOW()
);