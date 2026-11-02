<?php

declare(strict_types=1);

// This helper's stdout is a secret channel consumed by --token-command.
// Do not run it directly in a terminal or include its output in a report.
ini_set('display_errors', '0');
ini_set('log_errors', '0');
set_error_handler(static function (): never {
    throw new RuntimeException('Local credential unavailable.');
});
ob_start();

try {
    if ($argc < 2 || $argc > 3) {
        throw new RuntimeException();
    }
    $root = realpath($argv[1]);
    if ($root === false || !is_file($root.'/vendor/autoload.php')) {
        throw new RuntimeException();
    }
    require $root.'/vendor/autoload.php';

    if (is_file($root.'/bootstrap/cache/config.php')) {
        $config = require $root.'/bootstrap/cache/config.php';
        $connection = $config['database']['default'] ?? null;
        $database = $config['database']['connections']['sqlite']['database'] ?? null;
        $key = $config['app']['key'] ?? null;
        $cipher = $config['app']['cipher'] ?? 'AES-256-CBC';
        $previousKeys = $config['app']['previous_keys'] ?? [];
    } else {
        $env = Dotenv\Dotenv::parse(file_get_contents($root.'/.env'));
        $connection = $env['DB_CONNECTION'] ?? null;
        $database = $env['DB_DATABASE'] ?? $root.'/database/database.sqlite';
        $key = $env['APP_KEY'] ?? null;
        $cipher = $env['APP_CIPHER'] ?? 'AES-256-CBC';
        $previousKeys = array_filter(explode(',', $env['APP_PREVIOUS_KEYS'] ?? ''));
    }
    if ($connection !== 'sqlite' || !is_string($database) || !is_string($key)) {
        throw new RuntimeException();
    }
    if (!str_starts_with($database, '/')) {
        $database = $root.'/'.$database;
    }
    $database = realpath($database);
    if ($database === false || !is_file($database)) {
        throw new RuntimeException();
    }
    $uri = implode('/', array_map('rawurlencode', explode('/', $database)));
    $db = new PDO('sqlite:file:'.$uri.'?mode=ro', options: [PDO::ATTR_ERRMODE => PDO::ERRMODE_EXCEPTION]);
    $db->exec('PRAGMA query_only = ON');
    $sql = "SELECT token FROM agents WHERE connection_state = 'connected' AND token_version = 1";
    $parameters = [];
    if (isset($argv[2])) {
        if (!ctype_digit($argv[2]) || (int) $argv[2] < 1) {
            throw new RuntimeException();
        }
        $sql .= ' AND id = ?';
        $parameters[] = (int) $argv[2];
    }
    $statement = $db->prepare($sql.' LIMIT 2');
    $statement->execute($parameters);
    $rows = $statement->fetchAll(PDO::FETCH_COLUMN);
    if (count($rows) !== 1) {
        throw new RuntimeException();
    }
    $decode = static fn (string $value): string => str_starts_with($value, 'base64:')
        ? base64_decode(substr($value, 7), true) : $value;
    $crypt = new Illuminate\Encryption\Encrypter($decode($key), $cipher);
    $crypt->previousKeys(array_map($decode, $previousKeys));
    $token = $crypt->decrypt($rows[0], false);
    if (!is_string($token) || $token === '' || strlen($token) > 16384 || preg_match('/[^\x21-\x7E]/', $token)) {
        throw new RuntimeException();
    }
    ob_end_clean();
    echo $token;
} catch (Throwable) {
    ob_end_clean();
    fwrite(STDERR, "Local agent credential unavailable; check app path, SQLite configuration, key, and connected agent selection.\n");
    exit(1);
}
