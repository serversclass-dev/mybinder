-- ============================================================
-- SOUQIQ — Initial Database Migration
-- SQL Server 2022
-- ============================================================

USE souqiq;
GO

-- ── Cities ───────────────────────────────────────────────────
CREATE TABLE cities (
    id            NVARCHAR(36)  NOT NULL PRIMARY KEY,
    name          NVARCHAR(100) NOT NULL,
    name_ar       NVARCHAR(100) NOT NULL,
    country_code  CHAR(2)       NOT NULL DEFAULT 'LY',
    is_active     BIT           NOT NULL DEFAULT 1,
    created_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Users ────────────────────────────────────────────────────
CREATE TABLE users (
    id            NVARCHAR(36)  NOT NULL PRIMARY KEY,
    email         NVARCHAR(255) NULL,
    phone         NVARCHAR(30)  NOT NULL,
    password_hash NVARCHAR(MAX) NOT NULL,
    role          NVARCHAR(30)  NOT NULL,
    is_active     BIT           NOT NULL DEFAULT 1,
    is_verified   BIT           NOT NULL DEFAULT 0,
    last_login_at DATETIME2     NULL,
    created_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_users_phone EMAIL UNIQUE (phone),
    CONSTRAINT uq_users_email UNIQUE (email)
);
GO

CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_role  ON users(role);
GO

-- ── User Sessions ─────────────────────────────────────────────
CREATE TABLE user_sessions (
    id          NVARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id     NVARCHAR(36)  NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  NVARCHAR(MAX) NOT NULL,
    expires_at  DATETIME2     NOT NULL,
    ip_address  NVARCHAR(45)  NULL,
    user_agent  NVARCHAR(500) NULL,
    created_at  DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_sessions_user ON user_sessions(user_id);
GO

-- ── Suppliers ─────────────────────────────────────────────────
CREATE TABLE suppliers (
    id                       NVARCHAR(36)   NOT NULL PRIMARY KEY,
    user_id                  NVARCHAR(36)   NOT NULL REFERENCES users(id),
    business_name            NVARCHAR(255)  NOT NULL,
    business_name_ar         NVARCHAR(255)  NOT NULL,
    description              NVARCHAR(MAX)  NULL,
    logo_url                 NVARCHAR(MAX)  NULL,
    cover_url                NVARCHAR(MAX)  NULL,
    commercial_register      NVARCHAR(100)  NULL,
    tax_number               NVARCHAR(100)  NULL,
    status                   NVARCHAR(20)   NOT NULL DEFAULT 'pending',
    rejection_reason         NVARCHAR(MAX)  NULL,
    approved_by              NVARCHAR(36)   NULL REFERENCES users(id),
    approved_at              DATETIME2      NULL,
    commission_rate_override DECIMAL(5,2)   NULL,
    city_id                  NVARCHAR(36)   NULL REFERENCES cities(id),
    address                  NVARCHAR(MAX)  NULL,
    created_at               DATETIME2      NOT NULL DEFAULT GETUTCDATE(),
    updated_at               DATETIME2      NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_suppliers_user   ON suppliers(user_id);
CREATE INDEX idx_suppliers_status ON suppliers(status);
GO

-- ── Supplier Bank Accounts ────────────────────────────────────
CREATE TABLE supplier_bank_accounts (
    id             NVARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id    NVARCHAR(36)  NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    bank_name      NVARCHAR(100) NOT NULL,
    account_holder NVARCHAR(255) NOT NULL,
    account_number NVARCHAR(100) NOT NULL,
    iban           NVARCHAR(50)  NULL,
    is_primary     BIT           NOT NULL DEFAULT 0,
    created_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Supplier Documents ────────────────────────────────────────
CREATE TABLE supplier_documents (
    id            NVARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id   NVARCHAR(36)  NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    document_type NVARCHAR(50)  NOT NULL,
    file_url      NVARCHAR(MAX) NOT NULL,
    verified      BIT           NOT NULL DEFAULT 0,
    created_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Supplier Wallets ──────────────────────────────────────────
CREATE TABLE supplier_wallets (
    id                NVARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id       NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    balance_pending   DECIMAL(10,2) NOT NULL DEFAULT 0,
    balance_available DECIMAL(10,2) NOT NULL DEFAULT 0,
    balance_withdrawn DECIMAL(10,2) NOT NULL DEFAULT 0,
    last_updated_at   DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    created_at        DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at        DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_wallet_supplier UNIQUE (supplier_id)
);
GO

-- ── Customers ─────────────────────────────────────────────────
CREATE TABLE customers (
    id            NVARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id       NVARCHAR(36)  NOT NULL REFERENCES users(id),
    full_name     NVARCHAR(255) NOT NULL,
    avatar_url    NVARCHAR(MAX) NULL,
    date_of_birth DATE          NULL,
    created_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at    DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_customer_user UNIQUE (user_id)
);
GO

CREATE INDEX idx_customers_user ON customers(user_id);
GO

-- ── Customer Addresses ────────────────────────────────────────
CREATE TABLE customer_addresses (
    id           NVARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id  NVARCHAR(36)  NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    label        NVARCHAR(100) NOT NULL DEFAULT 'Home',
    full_address NVARCHAR(MAX) NOT NULL,
    city_id      NVARCHAR(36)  NOT NULL REFERENCES cities(id),
    district     NVARCHAR(100) NULL,
    latitude     DECIMAL(10,8) NULL,
    longitude    DECIMAL(11,8) NULL,
    is_default   BIT           NOT NULL DEFAULT 0,
    created_at   DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at   DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_addresses_customer ON customer_addresses(customer_id);
GO

-- ── Delivery Companies ────────────────────────────────────────
CREATE TABLE delivery_companies (
    id              NVARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id         NVARCHAR(36)  NOT NULL REFERENCES users(id),
    company_name    NVARCHAR(255) NOT NULL,
    company_name_ar NVARCHAR(255) NOT NULL,
    logo_url        NVARCHAR(MAX) NULL,
    status          NVARCHAR(20)  NOT NULL DEFAULT 'pending',
    approved_by     NVARCHAR(36)  NULL REFERENCES users(id),
    approved_at     DATETIME2     NULL,
    contact_email   NVARCHAR(255) NULL,
    contact_phone   NVARCHAR(30)  NULL,
    created_at      DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at      DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_delivery_user   ON delivery_companies(user_id);
CREATE INDEX idx_delivery_status ON delivery_companies(status);
GO

-- ── Delivery Zones ────────────────────────────────────────────
CREATE TABLE delivery_zones (
    id                  NVARCHAR(36)  NOT NULL PRIMARY KEY,
    delivery_company_id NVARCHAR(36)  NOT NULL REFERENCES delivery_companies(id) ON DELETE CASCADE,
    city_id             NVARCHAR(36)  NOT NULL REFERENCES cities(id),
    district            NVARCHAR(100) NULL,
    base_price          DECIMAL(10,2) NOT NULL DEFAULT 0,
    estimated_hours     INT           NOT NULL DEFAULT 24,
    is_active           BIT           NOT NULL DEFAULT 1,
    created_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_zones_company ON delivery_zones(delivery_company_id);
CREATE INDEX idx_zones_city    ON delivery_zones(city_id);
GO

-- ── Categories ────────────────────────────────────────────────
CREATE TABLE categories (
    id         NVARCHAR(36)  NOT NULL PRIMARY KEY,
    parent_id  NVARCHAR(36)  NULL REFERENCES categories(id),
    name       NVARCHAR(100) NOT NULL,
    name_ar    NVARCHAR(100) NOT NULL,
    slug       NVARCHAR(120) NOT NULL,
    icon_url   NVARCHAR(MAX) NULL,
    sort_order INT           NOT NULL DEFAULT 0,
    is_active  BIT           NOT NULL DEFAULT 1,
    created_at DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_category_slug UNIQUE (slug)
);
GO

-- ── Products ──────────────────────────────────────────────────
CREATE TABLE products (
    id                       NVARCHAR(36)   NOT NULL PRIMARY KEY,
    supplier_id              NVARCHAR(36)   NOT NULL REFERENCES suppliers(id),
    category_id              NVARCHAR(36)   NOT NULL REFERENCES categories(id),
    name                     NVARCHAR(255)  NOT NULL,
    name_ar                  NVARCHAR(255)  NOT NULL,
    description              NVARCHAR(MAX)  NULL,
    description_ar           NVARCHAR(MAX)  NULL,
    sku                      NVARCHAR(100)  NULL,
    status                   NVARCHAR(20)   NOT NULL DEFAULT 'draft',
    rejection_reason         NVARCHAR(MAX)  NULL,
    is_featured              BIT            NOT NULL DEFAULT 0,
    retail_price             DECIMAL(10,2)  NOT NULL,
    wholesale_price          DECIMAL(10,2)  NULL,
    wholesale_min_qty        INT            NULL,
    weight_kg                DECIMAL(8,3)   NULL,
    commission_rate_override DECIMAL(5,2)   NULL,
    created_at               DATETIME2      NOT NULL DEFAULT GETUTCDATE(),
    updated_at               DATETIME2      NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_products_supplier ON products(supplier_id);
CREATE INDEX idx_products_category ON products(category_id);
CREATE INDEX idx_products_status   ON products(status);
CREATE INDEX idx_products_featured ON products(is_featured);
GO

-- ── Product Images ────────────────────────────────────────────
CREATE TABLE product_images (
    id         NVARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id NVARCHAR(36)  NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url        NVARCHAR(MAX) NOT NULL,
    is_primary BIT           NOT NULL DEFAULT 0,
    sort_order INT           NOT NULL DEFAULT 0,
    created_at DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_images_product ON product_images(product_id);
GO

-- ── Product Attributes ────────────────────────────────────────
CREATE TABLE product_attributes (
    id              NVARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id      NVARCHAR(36)  NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    attribute_name  NVARCHAR(100) NOT NULL,
    attribute_value NVARCHAR(255) NOT NULL,
    created_at      DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at      DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Product Discounts ─────────────────────────────────────────
CREATE TABLE product_discounts (
    id             NVARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id     NVARCHAR(36)  NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    discount_type  NVARCHAR(20)  NOT NULL,
    discount_value DECIMAL(10,2) NOT NULL,
    start_at       DATETIME2     NOT NULL,
    end_at         DATETIME2     NULL,
    is_active      BIT           NOT NULL DEFAULT 1,
    created_by     NVARCHAR(36)  NOT NULL REFERENCES users(id),
    created_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Inventory ─────────────────────────────────────────────────
CREATE TABLE inventory (
    id                  NVARCHAR(36) NOT NULL PRIMARY KEY,
    product_id          NVARCHAR(36) NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity_available  INT          NOT NULL DEFAULT 0,
    quantity_reserved   INT          NOT NULL DEFAULT 0,
    low_stock_threshold INT          NOT NULL DEFAULT 5,
    allow_backorder     BIT          NOT NULL DEFAULT 0,
    created_at          DATETIME2    NOT NULL DEFAULT GETUTCDATE(),
    updated_at          DATETIME2    NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_inventory_product UNIQUE (product_id)
);
GO

-- ── Inventory Logs ────────────────────────────────────────────
CREATE TABLE inventory_logs (
    id               NVARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id       NVARCHAR(36)  NOT NULL REFERENCES products(id),
    change_type      NVARCHAR(20)  NOT NULL,
    quantity_before  INT           NOT NULL,
    quantity_change  INT           NOT NULL,
    quantity_after   INT           NOT NULL,
    reference_id     NVARCHAR(36)  NULL,
    note             NVARCHAR(500) NULL,
    performed_by     NVARCHAR(36)  NOT NULL,
    created_at       DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_inv_logs_product ON inventory_logs(product_id);
GO

-- ── Carts ─────────────────────────────────────────────────────
CREATE TABLE carts (
    id          NVARCHAR(36) NOT NULL PRIMARY KEY,
    customer_id NVARCHAR(36) NOT NULL REFERENCES customers(id),
    is_active   BIT          NOT NULL DEFAULT 1,
    expires_at  DATETIME2    NULL,
    created_at  DATETIME2    NOT NULL DEFAULT GETUTCDATE(),
    updated_at  DATETIME2    NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_carts_customer ON carts(customer_id);
GO

-- ── Cart Items ────────────────────────────────────────────────
CREATE TABLE cart_items (
    id             NVARCHAR(36)  NOT NULL PRIMARY KEY,
    cart_id        NVARCHAR(36)  NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    product_id     NVARCHAR(36)  NOT NULL REFERENCES products(id),
    supplier_id    NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    quantity       INT           NOT NULL DEFAULT 1,
    unit_price     DECIMAL(10,2) NOT NULL,
    is_wholesale   BIT           NOT NULL DEFAULT 0,
    price_snapshot NVARCHAR(MAX) NULL,
    created_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_cart_items_cart ON cart_items(cart_id);
GO

-- ── Wishlist ──────────────────────────────────────────────────
CREATE TABLE wishlist_items (
    id          NVARCHAR(36) NOT NULL PRIMARY KEY,
    customer_id NVARCHAR(36) NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    product_id  NVARCHAR(36) NOT NULL REFERENCES products(id),
    created_at  DATETIME2    NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_wishlist UNIQUE (customer_id, product_id)
);
GO

-- ── Orders ────────────────────────────────────────────────────
CREATE TABLE orders (
    id                  NVARCHAR(36)  NOT NULL PRIMARY KEY,
    order_number        NVARCHAR(30)  NOT NULL,
    customer_id         NVARCHAR(36)  NOT NULL REFERENCES customers(id),
    delivery_address_id NVARCHAR(36)  NOT NULL REFERENCES customer_addresses(id),
    status              NVARCHAR(30)  NOT NULL DEFAULT 'pending',
    payment_method      NVARCHAR(20)  NOT NULL DEFAULT 'cod',
    payment_status      NVARCHAR(30)  NOT NULL DEFAULT 'unpaid',
    subtotal            DECIMAL(10,2) NOT NULL DEFAULT 0,
    total_delivery_fee  DECIMAL(10,2) NOT NULL DEFAULT 0,
    total_commission    DECIMAL(10,2) NOT NULL DEFAULT 0,
    grand_total         DECIMAL(10,2) NOT NULL DEFAULT 0,
    notes               NVARCHAR(MAX) NULL,
    paid_at             DATETIME2     NULL,
    created_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_order_number UNIQUE (order_number)
);
GO

CREATE INDEX idx_orders_customer ON orders(customer_id);
CREATE INDEX idx_orders_status   ON orders(status);
CREATE INDEX idx_orders_number   ON orders(order_number);
GO

-- ── Sub Orders ────────────────────────────────────────────────
CREATE TABLE sub_orders (
    id                   NVARCHAR(36)  NOT NULL PRIMARY KEY,
    sub_order_number     NVARCHAR(40)  NOT NULL,
    order_id             NVARCHAR(36)  NOT NULL REFERENCES orders(id),
    supplier_id          NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    delivery_company_id  NVARCHAR(36)  NULL REFERENCES delivery_companies(id),
    status               NVARCHAR(30)  NOT NULL DEFAULT 'pending_supplier',
    delivery_fee         DECIMAL(10,2) NOT NULL DEFAULT 0,
    subtotal             DECIMAL(10,2) NOT NULL DEFAULT 0,
    commission_rate      DECIMAL(5,2)  NOT NULL DEFAULT 0,
    commission_amount    DECIMAL(10,2) NOT NULL DEFAULT 0,
    supplier_net         DECIMAL(10,2) NOT NULL DEFAULT 0,
    tracking_number      NVARCHAR(100) NULL,
    supplier_accepted_at DATETIME2     NULL,
    ready_at             DATETIME2     NULL,
    picked_up_at         DATETIME2     NULL,
    delivered_at         DATETIME2     NULL,
    rejection_reason     NVARCHAR(MAX) NULL,
    delivery_notes       NVARCHAR(MAX) NULL,
    assigned_by          NVARCHAR(36)  NULL REFERENCES users(id),
    created_at           DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at           DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_sub_order_number UNIQUE (sub_order_number)
);
GO

CREATE INDEX idx_sub_orders_order    ON sub_orders(order_id);
CREATE INDEX idx_sub_orders_supplier ON sub_orders(supplier_id);
CREATE INDEX idx_sub_orders_delivery ON sub_orders(delivery_company_id);
CREATE INDEX idx_sub_orders_status   ON sub_orders(status);
GO

-- ── Order Items ───────────────────────────────────────────────
CREATE TABLE order_items (
    id                    NVARCHAR(36)  NOT NULL PRIMARY KEY,
    sub_order_id          NVARCHAR(36)  NOT NULL REFERENCES sub_orders(id),
    order_id              NVARCHAR(36)  NOT NULL REFERENCES orders(id),
    product_id            NVARCHAR(36)  NOT NULL REFERENCES products(id),
    supplier_id           NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    product_name_snapshot NVARCHAR(255) NOT NULL,
    quantity              INT           NOT NULL DEFAULT 1,
    unit_price            DECIMAL(10,2) NOT NULL,
    is_wholesale          BIT           NOT NULL DEFAULT 0,
    line_total            DECIMAL(10,2) NOT NULL,
    discount_applied      DECIMAL(10,2) NOT NULL DEFAULT 0,
    created_at            DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at            DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_order_items_sub   ON order_items(sub_order_id);
CREATE INDEX idx_order_items_order ON order_items(order_id);
CREATE INDEX idx_order_items_prod  ON order_items(product_id);
GO

-- ── Order Status Logs ─────────────────────────────────────────
CREATE TABLE order_status_logs (
    id              NVARCHAR(36)  NOT NULL PRIMARY KEY,
    sub_order_id    NVARCHAR(36)  NOT NULL REFERENCES sub_orders(id),
    previous_status NVARCHAR(30)  NOT NULL DEFAULT '',
    new_status      NVARCHAR(30)  NOT NULL,
    changed_by      NVARCHAR(36)  NOT NULL,
    note            NVARCHAR(500) NULL,
    changed_at      DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_status_logs_sub ON order_status_logs(sub_order_id);
GO

-- ── Financial Transactions ────────────────────────────────────
CREATE TABLE financial_transactions (
    id                 NVARCHAR(36)  NOT NULL PRIMARY KEY,
    transaction_number NVARCHAR(40)  NOT NULL,
    type               NVARCHAR(30)  NOT NULL,
    direction          NVARCHAR(10)  NOT NULL,
    amount             DECIMAL(10,2) NOT NULL,
    currency           CHAR(3)       NOT NULL DEFAULT 'LYD',
    reference_type     NVARCHAR(50)  NULL,
    reference_id       NVARCHAR(36)  NULL,
    description        NVARCHAR(MAX) NULL,
    performed_by       NVARCHAR(36)  NULL,
    status             NVARCHAR(20)  NOT NULL DEFAULT 'completed',
    created_at         DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at         DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_txn_number UNIQUE (transaction_number)
);
GO

CREATE INDEX idx_txn_reference ON financial_transactions(reference_type, reference_id);
CREATE INDEX idx_txn_type      ON financial_transactions(type);
GO

-- ── Commission Records ────────────────────────────────────────
CREATE TABLE commission_records (
    id                     NVARCHAR(36)  NOT NULL PRIMARY KEY,
    sub_order_id           NVARCHAR(36)  NOT NULL REFERENCES sub_orders(id),
    supplier_id            NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    order_type             NVARCHAR(20)  NOT NULL DEFAULT 'retail',
    gross_amount           DECIMAL(10,2) NOT NULL,
    commission_rate        DECIMAL(5,2)  NOT NULL,
    commission_amount      DECIMAL(10,2) NOT NULL,
    platform_fee_from_buyer DECIMAL(10,2) NOT NULL DEFAULT 0,
    net_supplier_amount    DECIMAL(10,2) NOT NULL,
    settled                BIT           NOT NULL DEFAULT 0,
    settled_at             DATETIME2     NULL,
    created_at             DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at             DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_comm_supplier ON commission_records(supplier_id, settled);
CREATE INDEX idx_comm_suborder ON commission_records(sub_order_id);
GO

-- ── Withdrawal Requests ───────────────────────────────────────
CREATE TABLE withdrawal_requests (
    id               NVARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id      NVARCHAR(36)  NOT NULL REFERENCES suppliers(id),
    amount           DECIMAL(10,2) NOT NULL,
    bank_account_id  NVARCHAR(36)  NOT NULL REFERENCES supplier_bank_accounts(id),
    status           NVARCHAR(20)  NOT NULL DEFAULT 'pending',
    requested_at     DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    processed_by     NVARCHAR(36)  NULL REFERENCES users(id),
    processed_at     DATETIME2     NULL,
    rejection_reason NVARCHAR(MAX) NULL,
    proof_url        NVARCHAR(MAX) NULL,
    created_at       DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at       DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_withdrawals_supplier ON withdrawal_requests(supplier_id);
CREATE INDEX idx_withdrawals_status   ON withdrawal_requests(status);
GO

-- ── Reviews ───────────────────────────────────────────────────
CREATE TABLE reviews (
    id                  NVARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id         NVARCHAR(36)  NOT NULL REFERENCES customers(id),
    product_id          NVARCHAR(36)  NOT NULL REFERENCES products(id),
    sub_order_id        NVARCHAR(36)  NOT NULL REFERENCES sub_orders(id),
    rating              SMALLINT      NOT NULL CHECK (rating BETWEEN 1 AND 5),
    title               NVARCHAR(255) NULL,
    body                NVARCHAR(MAX) NULL,
    is_published        BIT           NOT NULL DEFAULT 1,
    supplier_reply      NVARCHAR(MAX) NULL,
    supplier_replied_at DATETIME2     NULL,
    created_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at          DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_review_per_purchase UNIQUE (customer_id, product_id, sub_order_id)
);
GO

CREATE INDEX idx_reviews_product  ON reviews(product_id);
CREATE INDEX idx_reviews_customer ON reviews(customer_id);
GO

-- ── Complaints ────────────────────────────────────────────────
CREATE TABLE complaints (
    id               NVARCHAR(36)  NOT NULL PRIMARY KEY,
    complaint_number NVARCHAR(30)  NOT NULL,
    raised_by        NVARCHAR(36)  NOT NULL REFERENCES users(id),
    raiser_role      NVARCHAR(20)  NOT NULL,
    sub_order_id     NVARCHAR(36)  NOT NULL REFERENCES sub_orders(id),
    type             NVARCHAR(30)  NOT NULL,
    description      NVARCHAR(MAX) NOT NULL,
    status           NVARCHAR(20)  NOT NULL DEFAULT 'open',
    resolution       NVARCHAR(MAX) NULL,
    resolved_by      NVARCHAR(36)  NULL REFERENCES users(id),
    resolved_at      DATETIME2     NULL,
    created_at       DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at       DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_complaint_number UNIQUE (complaint_number)
);
GO

CREATE INDEX idx_complaints_status   ON complaints(status);
CREATE INDEX idx_complaints_raised   ON complaints(raised_by);
CREATE INDEX idx_complaints_suborder ON complaints(sub_order_id);
GO

-- ── Complaint Attachments ─────────────────────────────────────
CREATE TABLE complaint_attachments (
    id           NVARCHAR(36)  NOT NULL PRIMARY KEY,
    complaint_id NVARCHAR(36)  NOT NULL REFERENCES complaints(id) ON DELETE CASCADE,
    file_url     NVARCHAR(MAX) NOT NULL,
    uploaded_by  NVARCHAR(36)  NOT NULL REFERENCES users(id),
    created_at   DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

-- ── Notifications ─────────────────────────────────────────────
CREATE TABLE notifications (
    id         NVARCHAR(36)   NOT NULL PRIMARY KEY,
    user_id    NVARCHAR(36)   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type       NVARCHAR(60)   NOT NULL,
    title      NVARCHAR(255)  NOT NULL,
    body       NVARCHAR(MAX)  NOT NULL,
    data       NVARCHAR(MAX)  NULL,
    is_read    BIT            NOT NULL DEFAULT 0,
    read_at    DATETIME2      NULL,
    created_at DATETIME2      NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_notif_user_unread ON notifications(user_id, is_read);
GO

-- ── Audit Logs ────────────────────────────────────────────────
CREATE TABLE audit_logs (
    id           NVARCHAR(36)  NOT NULL PRIMARY KEY,
    actor_id     NVARCHAR(36)  NOT NULL REFERENCES users(id),
    actor_role   NVARCHAR(50)  NOT NULL,
    action       NVARCHAR(100) NOT NULL,
    target_type  NVARCHAR(50)  NOT NULL,
    target_id    NVARCHAR(36)  NOT NULL,
    before_state NVARCHAR(MAX) NULL,
    after_state  NVARCHAR(MAX) NULL,
    ip_address   NVARCHAR(45)  NULL,
    performed_at DATETIME2     NOT NULL DEFAULT GETUTCDATE()
);
GO

CREATE INDEX idx_audit_target ON audit_logs(target_type, target_id);
CREATE INDEX idx_audit_actor  ON audit_logs(actor_id);
GO

-- ── Platform Settings ─────────────────────────────────────────
CREATE TABLE platform_settings (
    id          NVARCHAR(36)  NOT NULL PRIMARY KEY,
    [key]       NVARCHAR(100) NOT NULL,
    value       NVARCHAR(MAX) NOT NULL,
    value_type  NVARCHAR(20)  NOT NULL DEFAULT 'string',
    description NVARCHAR(MAX) NULL,
    updated_by  NVARCHAR(36)  NULL REFERENCES users(id),
    created_at  DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    updated_at  DATETIME2     NOT NULL DEFAULT GETUTCDATE(),
    CONSTRAINT uq_setting_key UNIQUE ([key])
);
GO

-- ── Seed: Cities ──────────────────────────────────────────────
INSERT INTO cities (id, name, name_ar, country_code, is_active) VALUES
('city-01', 'Tripoli',    N'طرابلس',    'LY', 1),
('city-02', 'Benghazi',   N'بنغازي',    'LY', 1),
('city-03', 'Misrata',    N'مصراتة',    'LY', 1),
('city-04', 'Bayda',      N'البيضاء',   'LY', 1),
('city-05', 'Zawiya',     N'الزاوية',   'LY', 1),
('city-06', 'Zliten',     N'زليتن',     'LY', 1),
('city-07', 'Zintan',     N'الزنتان',   'LY', 1),
('city-08', 'Sabah',      N'سبها',      'LY', 1),
('city-09', 'Derna',      N'درنة',      'LY', 1),
('city-10', 'Tobruk',     N'طبرق',      'LY', 1);
GO

-- ── Seed: Platform Settings ───────────────────────────────────
INSERT INTO platform_settings (id, [key], value, value_type, description) VALUES
(NEWID(), 'default_commission_rate',          '5.00',  'number', 'Default retail commission rate (%)'),
(NEWID(), 'wholesale_commission_rate',        '3.00',  'number', 'Default wholesale commission rate (%)'),
(NEWID(), 'wholesale_buyer_fee_rate',         '0.00',  'number', 'Buyer fee on wholesale orders (%)'),
(NEWID(), 'min_withdrawal_amount',            '50.00', 'number', 'Minimum supplier withdrawal amount (LYD)'),
(NEWID(), 'order_cancellation_window_minutes','30',    'number', 'Minutes customer can cancel after placing order'),
(NEWID(), 'auto_complete_order_hours',        '72',    'number', 'Hours after delivery before auto-confirmation'),
(NEWID(), 'supplier_acceptance_timeout_hours','24',    'number', 'Hours supplier has to accept a sub-order'),
(NEWID(), 'low_stock_threshold_default',      '5',     'number', 'Default low stock alert threshold (units)'),
(NEWID(), 'max_delivery_attempts',            '3',     'number', 'Max delivery attempts before admin intervention');
GO

-- ── Seed: Default Super Admin ─────────────────────────────────
-- Password: Admin@123456 (bcrypt hash — change immediately in production)
INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified) VALUES (
    NEWID(),
    '0910000000',
    'admin@souqiq.ly',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj/9p3pTq8Ei',
    'super_admin',
    1,
    1
);
GO

PRINT 'SOUQIQ database initialized successfully.';
GO