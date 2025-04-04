-- OllyStack Sample Database Schema
-- Orders and Inventory for sample microservices

-- Products table
CREATE TABLE IF NOT EXISTS products (
    id SERIAL PRIMARY KEY,
    sku VARCHAR(50) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Inventory table
CREATE TABLE IF NOT EXISTS inventory (
    id SERIAL PRIMARY KEY,
    product_id INTEGER REFERENCES products(id),
    quantity INTEGER NOT NULL DEFAULT 0,
    reserved INTEGER NOT NULL DEFAULT 0,
    warehouse VARCHAR(50) DEFAULT 'main',
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Orders table
CREATE TABLE IF NOT EXISTS orders (
    id SERIAL PRIMARY KEY,
    correlation_id VARCHAR(100),
    customer_id VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Order items table
CREATE TABLE IF NOT EXISTS order_items (
    id SERIAL PRIMARY KEY,
    order_id INTEGER REFERENCES orders(id),
    product_id INTEGER REFERENCES products(id),
    quantity INTEGER NOT NULL,
    unit_price DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Payments table
CREATE TABLE IF NOT EXISTS payments (
    id SERIAL PRIMARY KEY,
    order_id INTEGER REFERENCES orders(id),
    correlation_id VARCHAR(100),
    amount DECIMAL(10, 2) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    payment_method VARCHAR(50),
    transaction_id VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert sample products
INSERT INTO products (sku, name, description, price) VALUES
    ('LAPTOP-001', 'Gaming Laptop Pro', 'High-performance gaming laptop with RTX 4080', 1999.99),
    ('PHONE-001', 'SmartPhone X', 'Latest flagship smartphone with 5G', 999.99),
    ('HEADPHONE-001', 'Wireless Headphones Pro', 'Noise-canceling wireless headphones', 349.99),
    ('WATCH-001', 'Smart Watch Ultra', 'Premium smartwatch with health tracking', 499.99),
    ('TABLET-001', 'Pro Tablet 12', '12-inch professional tablet', 799.99),
    ('CAMERA-001', 'Mirrorless Camera', 'Professional mirrorless camera body', 2499.99),
    ('SPEAKER-001', 'Bluetooth Speaker', 'Portable Bluetooth speaker', 149.99),
    ('KEYBOARD-001', 'Mechanical Keyboard', 'RGB mechanical gaming keyboard', 199.99),
    ('MOUSE-001', 'Gaming Mouse', 'High-precision gaming mouse', 89.99),
    ('MONITOR-001', '4K Monitor', '32-inch 4K gaming monitor', 599.99)
ON CONFLICT (sku) DO NOTHING;

-- Initialize inventory for all products
INSERT INTO inventory (product_id, quantity, warehouse)
SELECT id, 100 + (random() * 900)::int, 'main'
FROM products
ON CONFLICT DO NOTHING;

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_orders_correlation_id ON orders(correlation_id);
CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders(customer_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_payments_correlation_id ON payments(correlation_id);
CREATE INDEX IF NOT EXISTS idx_inventory_product_id ON inventory(product_id);
