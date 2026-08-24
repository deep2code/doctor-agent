-- Runs once on first MariaDB init (docker-entrypoint-initdb.d).
-- Creates the two application databases used by doctor-agent.
CREATE DATABASE IF NOT EXISTS doctor_knowledge
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE DATABASE IF NOT EXISTS doctor_agent
  CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
