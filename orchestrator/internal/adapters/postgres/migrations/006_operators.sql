-- HU-05: operadores de la consola. Solo el hash bcrypt de la contraseña, nunca la contraseña.
CREATE TABLE operators (
    username      text PRIMARY KEY CHECK (length(username) BETWEEN 1 AND 100),
    display_name  text        NOT NULL,
    password_hash text        NOT NULL,
    active        boolean     NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);
