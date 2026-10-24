// Package domain содержит доменные порты: интерфейсы репозиториев, Unit of
// Work и доменные ошибки хранилища.
//
// Порты объявлены здесь, а не в инфраструктуре: пакет internal/model хранит
// сущности и value objects, а контракты доступа к ним — часть домена (см.
// docs/specification/programming.md §3.1). Реализации живут в
// internal/repository и не импортируются доменом.
package domain

import "errors"

// Доменные ошибки хранилища.
//
// Их возвращают репозитории (после трансляции ошибок драйвера) и проверяют
// сервисы и транспорт через errors.Is. Ошибки инфраструктуры — sql.ErrNoRows,
// SQLSTATE, коды SQLite — наружу из репозитория не выходят.
var (
	// ErrNotFound — запись не найдена.
	ErrNotFound = errors.New("record not found")
	// ErrDuplicate — нарушено ограничение уникальности (например, email занят).
	ErrDuplicate = errors.New("record already exists")
)
