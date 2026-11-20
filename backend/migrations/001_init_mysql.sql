SET FOREIGN_KEY_CHECKS = 0;

CREATE TABLE IF NOT EXISTS cities (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    name_ar      VARCHAR(100) NOT NULL,
    country_code CHAR(2)      NOT NULL DEFAULT 'LY',
    is_active    TINYINT(1)   NOT NULL DEFAULT 1,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS users (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    email         VARCHAR(255) NULL,
    phone         VARCHAR(30)  NOT NULL,
    password_hash TEXT         NOT NULL,
    role          VARCHAR(30)  NOT NULL,
    is_active     TINYINT(1)   NOT NULL DEFAULT 1,
    is_verified   TINYINT(1)   NOT NULL DEFAULT 0,
    last_login_at DATETIME     NULL,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_users_phone (phone),
    UNIQUE KEY uq_users_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_users_role ON users(role);

CREATE TABLE IF NOT EXISTS user_sessions (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id     VARCHAR(36)  NOT NULL,
    token_hash  TEXT         NOT NULL,
    expires_at  DATETIME     NOT NULL,
    ip_address  VARCHAR(45)  NULL,
    user_agent  VARCHAR(500) NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_sessions_user ON user_sessions(user_id);

CREATE TABLE IF NOT EXISTS suppliers (
    id                       VARCHAR(36)    NOT NULL PRIMARY KEY,
    user_id                  VARCHAR(36)    NOT NULL,
    business_name            VARCHAR(255)   NOT NULL,
    business_name_ar         VARCHAR(255)   NOT NULL,
    description              TEXT           NULL,
    logo_url                 TEXT           NULL,
    cover_url                TEXT           NULL,
    commercial_register      VARCHAR(100)   NULL,
    tax_number               VARCHAR(100)   NULL,
    status                   VARCHAR(20)    NOT NULL DEFAULT 'pending',
    rejection_reason         TEXT           NULL,
    approved_by              VARCHAR(36)    NULL,
    approved_at              DATETIME       NULL,
    commission_rate_override DECIMAL(5,2)   NULL,
    city_id                  VARCHAR(36)    NULL,
    address                  TEXT           NULL,
    created_at               DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id)    REFERENCES users(id),
    FOREIGN KEY (approved_by) REFERENCES users(id),
    FOREIGN KEY (city_id)    REFERENCES cities(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_suppliers_user   ON suppliers(user_id);
CREATE INDEX idx_suppliers_status ON suppliers(status);

CREATE TABLE IF NOT EXISTS supplier_bank_accounts (
    id             VARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id    VARCHAR(36)  NOT NULL,
    bank_name      VARCHAR(100) NOT NULL,
    account_holder VARCHAR(255) NOT NULL,
    account_number VARCHAR(100) NOT NULL,
    iban           VARCHAR(50)  NULL,
    is_primary     TINYINT(1)   NOT NULL DEFAULT 0,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS supplier_documents (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    supplier_id   VARCHAR(36)  NOT NULL,
    document_type VARCHAR(50)  NOT NULL,
    file_url      TEXT         NOT NULL,
    verified      TINYINT(1)   NOT NULL DEFAULT 0,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS supplier_wallets (
    id                VARCHAR(36)   NOT NULL PRIMARY KEY,
    supplier_id       VARCHAR(36)   NOT NULL,
    balance_pending   DECIMAL(10,2) NOT NULL DEFAULT 0,
    balance_available DECIMAL(10,2) NOT NULL DEFAULT 0,
    balance_withdrawn DECIMAL(10,2) NOT NULL DEFAULT 0,
    last_updated_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_wallet_supplier (supplier_id),
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS customers (
    id            VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id       VARCHAR(36)  NOT NULL,
    full_name     VARCHAR(255) NOT NULL,
    avatar_url    TEXT         NULL,
    date_of_birth DATE         NULL,
    created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_customer_user (user_id),
    FOREIGN KEY (user_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_customers_user ON customers(user_id);

CREATE TABLE IF NOT EXISTS customer_addresses (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id  VARCHAR(36)  NOT NULL,
    label        VARCHAR(100) NOT NULL DEFAULT 'Home',
    full_address TEXT         NOT NULL,
    city_id      VARCHAR(36)  NOT NULL,
    district     VARCHAR(100) NULL,
    latitude     DECIMAL(10,8) NULL,
    longitude    DECIMAL(11,8) NULL,
    is_default   TINYINT(1)   NOT NULL DEFAULT 0,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    FOREIGN KEY (city_id)     REFERENCES cities(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_addresses_customer ON customer_addresses(customer_id);

CREATE TABLE IF NOT EXISTS delivery_companies (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id         VARCHAR(36)  NOT NULL,
    company_name    VARCHAR(255) NOT NULL,
    company_name_ar VARCHAR(255) NOT NULL,
    logo_url        TEXT         NULL,
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    approved_by     VARCHAR(36)  NULL,
    approved_at     DATETIME     NULL,
    contact_email   VARCHAR(255) NULL,
    contact_phone   VARCHAR(30)  NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id)    REFERENCES users(id),
    FOREIGN KEY (approved_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_delivery_user   ON delivery_companies(user_id);
CREATE INDEX idx_delivery_status ON delivery_companies(status);

CREATE TABLE IF NOT EXISTS delivery_zones (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY,
    delivery_company_id VARCHAR(36)   NOT NULL,
    city_id             VARCHAR(36)   NOT NULL,
    district            VARCHAR(100)  NULL,
    base_price          DECIMAL(10,2) NOT NULL DEFAULT 0,
    estimated_hours     INT           NOT NULL DEFAULT 24,
    is_active           TINYINT(1)    NOT NULL DEFAULT 1,
    created_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (delivery_company_id) REFERENCES delivery_companies(id) ON DELETE CASCADE,
    FOREIGN KEY (city_id)             REFERENCES cities(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_zones_company ON delivery_zones(delivery_company_id);
CREATE INDEX idx_zones_city    ON delivery_zones(city_id);

CREATE TABLE IF NOT EXISTS categories (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    parent_id  VARCHAR(36)  NULL,
    name       VARCHAR(100) NOT NULL,
    name_ar    VARCHAR(100) NOT NULL,
    slug       VARCHAR(120) NOT NULL,
    icon_url   TEXT         NULL,
    sort_order INT          NOT NULL DEFAULT 0,
    is_active  TINYINT(1)   NOT NULL DEFAULT 1,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_category_slug (slug),
    FOREIGN KEY (parent_id) REFERENCES categories(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS products (
    id                       VARCHAR(36)   NOT NULL PRIMARY KEY,
    supplier_id              VARCHAR(36)   NOT NULL,
    category_id              VARCHAR(36)   NOT NULL,
    name                     VARCHAR(255)  NOT NULL,
    name_ar                  VARCHAR(255)  NOT NULL,
    description              TEXT          NULL,
    description_ar           TEXT          NULL,
    sku                      VARCHAR(100)  NULL,
    status                   VARCHAR(20)   NOT NULL DEFAULT 'draft',
    rejection_reason         TEXT          NULL,
    is_featured              TINYINT(1)    NOT NULL DEFAULT 0,
    retail_price             DECIMAL(10,2) NOT NULL,
    wholesale_price          DECIMAL(10,2) NULL,
    wholesale_min_qty        INT           NULL,
    weight_kg                DECIMAL(8,3)  NULL,
    commission_rate_override DECIMAL(5,2)  NULL,
    created_at               DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at               DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (supplier_id)  REFERENCES suppliers(id),
    FOREIGN KEY (category_id)  REFERENCES categories(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_products_supplier ON products(supplier_id);
CREATE INDEX idx_products_category ON products(category_id);
CREATE INDEX idx_products_status   ON products(status);
CREATE INDEX idx_products_featured ON products(is_featured);

CREATE TABLE IF NOT EXISTS product_images (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id VARCHAR(36)  NOT NULL,
    url        TEXT         NOT NULL,
    is_primary TINYINT(1)   NOT NULL DEFAULT 0,
    sort_order INT          NOT NULL DEFAULT 0,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_images_product ON product_images(product_id);

CREATE TABLE IF NOT EXISTS product_attributes (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id      VARCHAR(36)  NOT NULL,
    attribute_name  VARCHAR(100) NOT NULL,
    attribute_value VARCHAR(255) NOT NULL,
    created_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS product_discounts (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY,
    product_id     VARCHAR(36)   NOT NULL,
    discount_type  VARCHAR(20)   NOT NULL,
    discount_value DECIMAL(10,2) NOT NULL,
    start_at       DATETIME      NOT NULL,
    end_at         DATETIME      NULL,
    is_active      TINYINT(1)    NOT NULL DEFAULT 1,
    created_by     VARCHAR(36)   NOT NULL,
    created_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS inventory (
    id                  VARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id          VARCHAR(36)  NOT NULL,
    quantity_available  INT          NOT NULL DEFAULT 0,
    quantity_reserved   INT          NOT NULL DEFAULT 0,
    low_stock_threshold INT          NOT NULL DEFAULT 5,
    allow_backorder     TINYINT(1)   NOT NULL DEFAULT 0,
    created_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_inventory_product (product_id),
    FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS inventory_logs (
    id               VARCHAR(36)  NOT NULL PRIMARY KEY,
    product_id       VARCHAR(36)  NOT NULL,
    change_type      VARCHAR(20)  NOT NULL,
    quantity_before  INT          NOT NULL,
    quantity_change  INT          NOT NULL,
    quantity_after   INT          NOT NULL,
    reference_id     VARCHAR(36)  NULL,
    note             VARCHAR(500) NULL,
    performed_by     VARCHAR(36)  NOT NULL,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_inv_logs_product ON inventory_logs(product_id);

CREATE TABLE IF NOT EXISTS carts (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id VARCHAR(36)  NOT NULL,
    is_active   TINYINT(1)   NOT NULL DEFAULT 1,
    expires_at  DATETIME     NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (customer_id) REFERENCES customers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_carts_customer ON carts(customer_id);

CREATE TABLE IF NOT EXISTS cart_items (
    id             VARCHAR(36)   NOT NULL PRIMARY KEY,
    cart_id        VARCHAR(36)   NOT NULL,
    product_id     VARCHAR(36)   NOT NULL,
    supplier_id    VARCHAR(36)   NOT NULL,
    quantity       INT           NOT NULL DEFAULT 1,
    unit_price     DECIMAL(10,2) NOT NULL,
    is_wholesale   TINYINT(1)    NOT NULL DEFAULT 0,
    price_snapshot TEXT          NULL,
    created_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (cart_id)    REFERENCES carts(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id) REFERENCES products(id),
    FOREIGN KEY (supplier_id) REFERENCES suppliers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_cart_items_cart ON cart_items(cart_id);

CREATE TABLE IF NOT EXISTS wishlist_items (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id VARCHAR(36)  NOT NULL,
    product_id  VARCHAR(36)  NOT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_wishlist (customer_id, product_id),
    FOREIGN KEY (customer_id) REFERENCES customers(id) ON DELETE CASCADE,
    FOREIGN KEY (product_id)  REFERENCES products(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS orders (
    id                  VARCHAR(36)   NOT NULL PRIMARY KEY,
    order_number        VARCHAR(30)   NOT NULL,
    customer_id         VARCHAR(36)   NOT NULL,
    delivery_address_id VARCHAR(36)   NOT NULL,
    status              VARCHAR(30)   NOT NULL DEFAULT 'pending',
    payment_method      VARCHAR(20)   NOT NULL DEFAULT 'cod',
    payment_status      VARCHAR(30)   NOT NULL DEFAULT 'unpaid',
    subtotal            DECIMAL(10,2) NOT NULL DEFAULT 0,
    total_delivery_fee  DECIMAL(10,2) NOT NULL DEFAULT 0,
    total_commission    DECIMAL(10,2) NOT NULL DEFAULT 0,
    grand_total         DECIMAL(10,2) NOT NULL DEFAULT 0,
    notes               TEXT          NULL,
    paid_at             DATETIME      NULL,
    created_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_order_number (order_number),
    FOREIGN KEY (customer_id)         REFERENCES customers(id),
    FOREIGN KEY (delivery_address_id) REFERENCES customer_addresses(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_orders_customer ON orders(customer_id);
CREATE INDEX idx_orders_status   ON orders(status);

CREATE TABLE IF NOT EXISTS sub_orders (
    id                   VARCHAR(36)   NOT NULL PRIMARY KEY,
    sub_order_number     VARCHAR(40)   NOT NULL,
    order_id             VARCHAR(36)   NOT NULL,
    supplier_id          VARCHAR(36)   NOT NULL,
    delivery_company_id  VARCHAR(36)   NULL,
    status               VARCHAR(30)   NOT NULL DEFAULT 'pending_supplier',
    delivery_fee         DECIMAL(10,2) NOT NULL DEFAULT 0,
    subtotal             DECIMAL(10,2) NOT NULL DEFAULT 0,
    commission_rate      DECIMAL(5,2)  NOT NULL DEFAULT 0,
    commission_amount    DECIMAL(10,2) NOT NULL DEFAULT 0,
    supplier_net         DECIMAL(10,2) NOT NULL DEFAULT 0,
    tracking_number      VARCHAR(100)  NULL,
    supplier_accepted_at DATETIME      NULL,
    ready_at             DATETIME      NULL,
    picked_up_at         DATETIME      NULL,
    delivered_at         DATETIME      NULL,
    rejection_reason     TEXT          NULL,
    delivery_notes       TEXT          NULL,
    assigned_by          VARCHAR(36)   NULL,
    created_at           DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_sub_order_number (sub_order_number),
    FOREIGN KEY (order_id)            REFERENCES orders(id),
    FOREIGN KEY (supplier_id)         REFERENCES suppliers(id),
    FOREIGN KEY (delivery_company_id) REFERENCES delivery_companies(id),
    FOREIGN KEY (assigned_by)         REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_sub_orders_order    ON sub_orders(order_id);
CREATE INDEX idx_sub_orders_supplier ON sub_orders(supplier_id);
CREATE INDEX idx_sub_orders_delivery ON sub_orders(delivery_company_id);
CREATE INDEX idx_sub_orders_status   ON sub_orders(status);

CREATE TABLE IF NOT EXISTS order_items (
    id                    VARCHAR(36)   NOT NULL PRIMARY KEY,
    sub_order_id          VARCHAR(36)   NOT NULL,
    order_id              VARCHAR(36)   NOT NULL,
    product_id            VARCHAR(36)   NOT NULL,
    supplier_id           VARCHAR(36)   NOT NULL,
    product_name_snapshot VARCHAR(255)  NOT NULL,
    quantity              INT           NOT NULL DEFAULT 1,
    unit_price            DECIMAL(10,2) NOT NULL,
    is_wholesale          TINYINT(1)    NOT NULL DEFAULT 0,
    line_total            DECIMAL(10,2) NOT NULL,
    discount_applied      DECIMAL(10,2) NOT NULL DEFAULT 0,
    created_at            DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at            DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (sub_order_id) REFERENCES sub_orders(id),
    FOREIGN KEY (order_id)     REFERENCES orders(id),
    FOREIGN KEY (product_id)   REFERENCES products(id),
    FOREIGN KEY (supplier_id)  REFERENCES suppliers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_order_items_sub   ON order_items(sub_order_id);
CREATE INDEX idx_order_items_order ON order_items(order_id);
CREATE INDEX idx_order_items_prod  ON order_items(product_id);

CREATE TABLE IF NOT EXISTS order_status_logs (
    id              VARCHAR(36)  NOT NULL PRIMARY KEY,
    sub_order_id    VARCHAR(36)  NOT NULL,
    previous_status VARCHAR(30)  NOT NULL DEFAULT '',
    new_status      VARCHAR(30)  NOT NULL,
    changed_by      VARCHAR(36)  NOT NULL,
    note            VARCHAR(500) NULL,
    changed_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (sub_order_id) REFERENCES sub_orders(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_status_logs_sub ON order_status_logs(sub_order_id);

CREATE TABLE IF NOT EXISTS financial_transactions (
    id                 VARCHAR(36)   NOT NULL PRIMARY KEY,
    transaction_number VARCHAR(40)   NOT NULL,
    type               VARCHAR(30)   NOT NULL,
    direction          VARCHAR(10)   NOT NULL,
    amount             DECIMAL(10,2) NOT NULL,
    currency           CHAR(3)       NOT NULL DEFAULT 'LYD',
    reference_type     VARCHAR(50)   NULL,
    reference_id       VARCHAR(36)   NULL,
    description        TEXT          NULL,
    performed_by       VARCHAR(36)   NULL,
    status             VARCHAR(20)   NOT NULL DEFAULT 'completed',
    created_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_txn_number (transaction_number)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_txn_reference ON financial_transactions(reference_type, reference_id);
CREATE INDEX idx_txn_type      ON financial_transactions(type);

CREATE TABLE IF NOT EXISTS commission_records (
    id                      VARCHAR(36)   NOT NULL PRIMARY KEY,
    sub_order_id            VARCHAR(36)   NOT NULL,
    supplier_id             VARCHAR(36)   NOT NULL,
    order_type              VARCHAR(20)   NOT NULL DEFAULT 'retail',
    gross_amount            DECIMAL(10,2) NOT NULL,
    commission_rate         DECIMAL(5,2)  NOT NULL,
    commission_amount       DECIMAL(10,2) NOT NULL,
    platform_fee_from_buyer DECIMAL(10,2) NOT NULL DEFAULT 0,
    net_supplier_amount     DECIMAL(10,2) NOT NULL,
    settled                 TINYINT(1)    NOT NULL DEFAULT 0,
    settled_at              DATETIME      NULL,
    created_at              DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at              DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (sub_order_id) REFERENCES sub_orders(id),
    FOREIGN KEY (supplier_id)  REFERENCES suppliers(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_comm_supplier ON commission_records(supplier_id, settled);
CREATE INDEX idx_comm_suborder ON commission_records(sub_order_id);

CREATE TABLE IF NOT EXISTS withdrawal_requests (
    id               VARCHAR(36)   NOT NULL PRIMARY KEY,
    supplier_id      VARCHAR(36)   NOT NULL,
    amount           DECIMAL(10,2) NOT NULL,
    bank_account_id  VARCHAR(36)   NOT NULL,
    status           VARCHAR(20)   NOT NULL DEFAULT 'pending',
    requested_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processed_by     VARCHAR(36)   NULL,
    processed_at     DATETIME      NULL,
    rejection_reason TEXT          NULL,
    proof_url        TEXT          NULL,
    created_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (supplier_id)     REFERENCES suppliers(id),
    FOREIGN KEY (bank_account_id) REFERENCES supplier_bank_accounts(id),
    FOREIGN KEY (processed_by)    REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_withdrawals_supplier ON withdrawal_requests(supplier_id);
CREATE INDEX idx_withdrawals_status   ON withdrawal_requests(status);

CREATE TABLE IF NOT EXISTS reviews (
    id                   VARCHAR(36)  NOT NULL PRIMARY KEY,
    customer_id          VARCHAR(36)  NOT NULL,
    product_id           VARCHAR(36)  NOT NULL,
    sub_order_id         VARCHAR(36)  NOT NULL,
    rating               TINYINT      NOT NULL CHECK (rating BETWEEN 1 AND 5),
    title                VARCHAR(255) NULL,
    body                 TEXT         NULL,
    is_published         TINYINT(1)   NOT NULL DEFAULT 1,
    supplier_reply       TEXT         NULL,
    supplier_replied_at  DATETIME     NULL,
    created_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_review_per_purchase (customer_id, product_id, sub_order_id),
    FOREIGN KEY (customer_id)  REFERENCES customers(id),
    FOREIGN KEY (product_id)   REFERENCES products(id),
    FOREIGN KEY (sub_order_id) REFERENCES sub_orders(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_reviews_product  ON reviews(product_id);
CREATE INDEX idx_reviews_customer ON reviews(customer_id);

CREATE TABLE IF NOT EXISTS complaints (
    id               VARCHAR(36)  NOT NULL PRIMARY KEY,
    complaint_number VARCHAR(30)  NOT NULL,
    raised_by        VARCHAR(36)  NOT NULL,
    raiser_role      VARCHAR(20)  NOT NULL,
    sub_order_id     VARCHAR(36)  NOT NULL,
    type             VARCHAR(30)  NOT NULL,
    description      TEXT         NOT NULL,
    status           VARCHAR(20)  NOT NULL DEFAULT 'open',
    resolution       TEXT         NULL,
    resolved_by      VARCHAR(36)  NULL,
    resolved_at      DATETIME     NULL,
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_complaint_number (complaint_number),
    FOREIGN KEY (raised_by)    REFERENCES users(id),
    FOREIGN KEY (sub_order_id) REFERENCES sub_orders(id),
    FOREIGN KEY (resolved_by)  REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_complaints_status   ON complaints(status);
CREATE INDEX idx_complaints_raised   ON complaints(raised_by);
CREATE INDEX idx_complaints_suborder ON complaints(sub_order_id);

CREATE TABLE IF NOT EXISTS complaint_attachments (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    complaint_id VARCHAR(36)  NOT NULL,
    file_url     TEXT         NOT NULL,
    uploaded_by  VARCHAR(36)  NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (complaint_id) REFERENCES complaints(id) ON DELETE CASCADE,
    FOREIGN KEY (uploaded_by)  REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS notifications (
    id         VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id    VARCHAR(36)  NOT NULL,
    type       VARCHAR(60)  NOT NULL,
    title      VARCHAR(255) NOT NULL,
    body       TEXT         NOT NULL,
    data       TEXT         NULL,
    is_read    TINYINT(1)   NOT NULL DEFAULT 0,
    read_at    DATETIME     NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_notif_user_unread ON notifications(user_id, is_read);

CREATE TABLE IF NOT EXISTS audit_logs (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    actor_id     VARCHAR(36)  NOT NULL,
    actor_role   VARCHAR(50)  NOT NULL,
    action       VARCHAR(100) NOT NULL,
    target_type  VARCHAR(50)  NOT NULL,
    target_id    VARCHAR(36)  NOT NULL,
    before_state TEXT         NULL,
    after_state  TEXT         NULL,
    ip_address   VARCHAR(45)  NULL,
    performed_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (actor_id) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE INDEX idx_audit_target ON audit_logs(target_type, target_id);
CREATE INDEX idx_audit_actor  ON audit_logs(actor_id);

CREATE TABLE IF NOT EXISTS platform_settings (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    `key`       VARCHAR(100) NOT NULL,
    value       TEXT         NOT NULL,
    value_type  VARCHAR(20)  NOT NULL DEFAULT 'string',
    description TEXT         NULL,
    updated_by  VARCHAR(36)  NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_setting_key (`key`),
    FOREIGN KEY (updated_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;

-- ── Seed: Cities ──────────────────────────────────────────────────────────────
INSERT INTO cities (id, name, name_ar, country_code, is_active) VALUES
('city-01', 'Tripoli',  'طرابلس',  'LY', 1),
('city-02', 'Benghazi', 'بنغازي',  'LY', 1),
('city-03', 'Misrata',  'مصراتة',  'LY', 1),
('city-04', 'Bayda',    'البيضاء', 'LY', 1),
('city-05', 'Zawiya',   'الزاوية', 'LY', 1),
('city-06', 'Zliten',   'زليتن',   'LY', 1),
('city-07', 'Zintan',   'الزنتان', 'LY', 1),
('city-08', 'Sabha',    'سبها',    'LY', 1),
('city-09', 'Derna',    'درنة',    'LY', 1),
('city-10', 'Tobruk',   'طبرق',    'LY', 1);

-- ── Seed: Platform Settings ───────────────────────────────────────────────────
INSERT INTO platform_settings (id, `key`, value, value_type, description) VALUES
(UUID(), 'default_commission_rate',           '5.00',  'number', 'Default retail commission rate (%)'),
(UUID(), 'wholesale_commission_rate',         '3.00',  'number', 'Default wholesale commission rate (%)'),
(UUID(), 'wholesale_buyer_fee_rate',          '0.00',  'number', 'Buyer fee on wholesale orders (%)'),
(UUID(), 'min_withdrawal_amount',             '50.00', 'number', 'Minimum supplier withdrawal amount (LYD)'),
(UUID(), 'order_cancellation_window_minutes', '30',    'number', 'Minutes customer can cancel after placing order'),
(UUID(), 'auto_complete_order_hours',         '72',    'number', 'Hours after delivery before auto-confirmation'),
(UUID(), 'supplier_acceptance_timeout_hours', '24',    'number', 'Hours supplier has to accept a sub-order'),
(UUID(), 'low_stock_threshold_default',       '5',     'number', 'Default low stock alert threshold (units)'),
(UUID(), 'max_delivery_attempts',             '3',     'number', 'Max delivery attempts before admin intervention');

-- ── Seed: Default Super Admin ─────────────────────────────────────────────────
-- Password: Admin@123456  (bcrypt hash — change in production!)
INSERT INTO users (id, phone, email, password_hash, role, is_active, is_verified) VALUES (
    UUID(),
    '0910000000',
    'admin@souqiq.ly',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewdBPj/9p3pTq8Ei',
    'super_admin',
    1,
    1
);