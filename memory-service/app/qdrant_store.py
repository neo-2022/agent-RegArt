"""Совместимый с текущей логикой memory-service слой коллекции поверх Qdrant."""

from __future__ import annotations

from typing import Any, Dict, Iterable, List, Optional

from qdrant_client import QdrantClient
from qdrant_client.http import models


class QdrantCollectionCompat:
    """
    Адаптер, который предоставляет совместимый API коллекции для memory-service.

    Нужен для быстрой миграции бизнес-логики memory-service на Qdrant без тотального
    переписывания всех use-case методов в один шаг.
    """

    def __init__(self, client: QdrantClient, name: str, vector_size: int):
        self.client = client
        self.name = name
        self.vector_size = vector_size
        self._ensure_collection(vector_size)

    def _ensure_collection(self, vector_size: int) -> None:
        existing = [item.name for item in self.client.get_collections().collections]
        if self.name in existing:
            return
        self.client.create_collection(
            collection_name=self.name,
            vectors_config=models.VectorParams(size=vector_size, distance=models.Distance.COSINE),
        )

    @staticmethod
    def _build_filter(where: Optional[Dict[str, Any]]) -> Optional[models.Filter]:
        if not where:
            return None
        must = QdrantCollectionCompat._flatten_conditions(where)
        return models.Filter(must=must) if must else None

    @staticmethod
    def _flatten_conditions(where: Dict[str, Any]) -> List[models.FieldCondition]:
        conditions: List[models.FieldCondition] = []
        for key, value in where.items():
            if key == "$and" and isinstance(value, list):
                for nested in value:
                    if isinstance(nested, dict):
                        conditions.extend(QdrantCollectionCompat._flatten_conditions(nested))
                continue
            conditions.append(models.FieldCondition(key=f"meta.{key}", match=models.MatchValue(value=value)))
        return conditions

    @staticmethod
    def _payload_to_doc_meta(payload: Dict[str, Any]) -> tuple[str, Dict[str, Any]]:
        return str(payload.get("document", "")), dict(payload.get("meta", {}) or {})

    def _normalize_vector(self, vector: List[float]) -> List[float]:
        if len(vector) == self.vector_size:
            return vector
        if len(vector) > self.vector_size:
            return vector[: self.vector_size]
        return vector + [0.0] * (self.vector_size - len(vector))

    def add(
        self,
        documents: List[str],
        metadatas: List[Dict[str, Any]],
        ids: List[str],
        embeddings: Optional[List[List[float]]] = None,
    ) -> None:
        vectors = embeddings or [[0.0] * self.vector_size for _ in ids]
        points = [
            models.PointStruct(
                id=ids[idx],
                vector=self._normalize_vector(vectors[idx]),
                payload={"document": documents[idx], "meta": metadatas[idx]},
            )
            for idx in range(len(ids))
        ]
        self.client.upsert(collection_name=self.name, points=points, wait=True)

    def count(self) -> int:
        return int(self.client.count(collection_name=self.name, exact=True).count)

    def get(
        self,
        ids: Optional[List[str]] = None,
        where: Optional[Dict[str, Any]] = None,
        include: Optional[Iterable[str]] = None,
    ) -> Dict[str, Any]:
        del include
        points: List[models.Record]
        if ids:
            points = self.client.retrieve(collection_name=self.name, ids=ids, with_payload=True, with_vectors=False)
        else:
            filt = self._build_filter(where)
            points, _ = self.client.scroll(
                collection_name=self.name,
                scroll_filter=filt,
                with_payload=True,
                with_vectors=False,
                limit=max(self.count(), 1),
            )

        out_ids: List[str] = []
        out_docs: List[str] = []
        out_meta: List[Dict[str, Any]] = []
        for point in points:
            payload = dict(point.payload or {})
            doc, meta = self._payload_to_doc_meta(payload)
            out_ids.append(str(point.id))
            out_docs.append(doc)
            out_meta.append(meta)
        return {"ids": out_ids, "documents": out_docs, "metadatas": out_meta}

    def query(
        self,
        query_embeddings: List[List[float]],
        n_results: int,
        where: Optional[Dict[str, Any]] = None,
        include: Optional[Iterable[str]] = None,
    ) -> Dict[str, Any]:
        del include
        filt = self._build_filter(where)
        vector = query_embeddings[0]
        hits = self.client.search(
            collection_name=self.name,
            query_vector=vector,
            query_filter=filt,
            limit=max(n_results, 1),
            with_payload=True,
            with_vectors=False,
        )

        ids: List[str] = []
        docs: List[str] = []
        metas: List[Dict[str, Any]] = []
        distances: List[float] = []
        for hit in hits:
            payload = dict(hit.payload or {})
            doc, meta = self._payload_to_doc_meta(payload)
            ids.append(str(hit.id))
            docs.append(doc)
            metas.append(meta)
            # Совместимость с прежним кодом: он ожидает "distance" (меньше — лучше).
            distances.append(max(0.0, 1.0 - float(hit.score)))

        return {
            "ids": [ids],
            "documents": [docs],
            "metadatas": [metas],
            "distances": [distances],
        }

    def update(self, ids: List[str], metadatas: List[Dict[str, Any]]) -> None:
        current = self.get(ids=ids)
        cur_docs = current.get("documents", [])
        points: List[models.PointStruct] = []
        for idx, point_id in enumerate(ids):
            existing = self.client.retrieve(collection_name=self.name, ids=[point_id], with_vectors=True, with_payload=False)
            if not existing:
                continue
            vector = existing[0].vector
            points.append(
                models.PointStruct(
                    id=point_id,
                    vector=vector,
                    payload={"document": cur_docs[idx] if idx < len(cur_docs) else "", "meta": metadatas[idx]},
                )
            )
        if points:
            self.client.upsert(collection_name=self.name, points=points, wait=True)

    def delete(self, ids: List[str]) -> None:
        self.client.delete(
            collection_name=self.name,
            points_selector=models.PointIdsList(points=ids),
            wait=True,
        )
