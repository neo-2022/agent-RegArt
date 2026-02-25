"""Адаптер коллекций Qdrant с интерфейсом, близким к прежнему storage-слою memory-service."""

from __future__ import annotations

import uuid
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Optional

from qdrant_client import QdrantClient
from qdrant_client.http import models as qm


@dataclass
class QdrantCollection:
    """Лёгкая обёртка над одной коллекцией Qdrant."""

    client: QdrantClient
    name: str

    def add(self, embeddings: List[List[float]], documents: List[str], metadatas: List[Dict[str, Any]], ids: List[str]) -> None:
        points = [
            qm.PointStruct(
                id=pid,
                vector=embeddings[idx],
                payload={"document": documents[idx], "metadata": metadatas[idx]},
            )
            for idx, pid in enumerate(ids)
        ]
        self.client.upsert(collection_name=self.name, points=points)

    def count(self) -> int:
        return int(self.client.count(collection_name=self.name, exact=True).count)

    def query(self, query_embeddings: List[List[float]], n_results: int, where: Optional[Dict[str, Any]] = None,
              include: Optional[List[str]] = None) -> Dict[str, Any]:
        include = include or ["documents", "distances", "metadatas"]
        must = _to_qdrant_filter(where)
        qfilter = qm.Filter(must=must) if must else None
        docs: List[List[str]] = [[]]
        dists: List[List[float]] = [[]]
        metas: List[List[Dict[str, Any]]] = [[]]

        results = self.client.search(
            collection_name=self.name,
            query_vector=query_embeddings[0],
            query_filter=qfilter,
            limit=max(1, n_results),
            with_payload=True,
        )
        for p in results:
            payload = p.payload or {}
            docs[0].append(str(payload.get("document", "")))
            dists[0].append(float(p.score))
            metas[0].append(_ensure_dict(payload.get("metadata")))

        out: Dict[str, Any] = {}
        if "documents" in include:
            out["documents"] = docs
        if "distances" in include:
            # Для совместимости с прежней логикой Chroma в memory.py: меньше = лучше.
            out["distances"] = [[max(0.0, 1.0 - score) for score in dists[0]]]
        if "metadatas" in include:
            out["metadatas"] = metas
        return out

    def get(self, ids: Optional[List[str]] = None, where: Optional[Dict[str, Any]] = None,
            include: Optional[List[str]] = None) -> Dict[str, Any]:
        include = include or ["metadatas", "documents"]

        points = []
        if ids:
            points = self.client.retrieve(collection_name=self.name, ids=ids, with_payload=True)
        else:
            must = _to_qdrant_filter(where)
            qfilter = qm.Filter(must=must) if must else None
            scroll, _ = self.client.scroll(
                collection_name=self.name,
                scroll_filter=qfilter,
                with_payload=True,
                limit=10000,
            )
            points = scroll

        out_ids: List[str] = []
        docs: List[str] = []
        metas: List[Dict[str, Any]] = []
        for p in points:
            out_ids.append(str(p.id))
            payload = p.payload or {}
            docs.append(str(payload.get("document", "")))
            metas.append(_ensure_dict(payload.get("metadata")))

        out: Dict[str, Any] = {"ids": out_ids}
        if "documents" in include:
            out["documents"] = docs
        if "metadatas" in include:
            out["metadatas"] = metas
        return out

    def update(self, ids: List[str], metadatas: List[Dict[str, Any]]) -> None:
        if not ids:
            return
        points = self.client.retrieve(collection_name=self.name, ids=ids, with_payload=True, with_vectors=True)
        update_points = []
        for p in points:
            idx = ids.index(str(p.id))
            payload = p.payload or {}
            payload["metadata"] = metadatas[idx]
            update_points.append(qm.PointStruct(id=p.id, vector=p.vector, payload=payload))
        if update_points:
            self.client.upsert(collection_name=self.name, points=update_points)

    def delete(self, ids: List[str]) -> None:
        if not ids:
            return
        self.client.delete(
            collection_name=self.name,
            points_selector=qm.PointIdsList(points=ids),
        )


def ensure_collection(client: QdrantClient, name: str, vector_size: int) -> QdrantCollection:
    existing = [col.name for col in client.get_collections().collections]
    if name not in existing:
        client.create_collection(
            collection_name=name,
            vectors_config=qm.VectorParams(size=vector_size, distance=qm.Distance.COSINE),
        )
    return QdrantCollection(client=client, name=name)


def _ensure_dict(value: Any) -> Dict[str, Any]:
    return value if isinstance(value, dict) else {}


def _to_qdrant_filter(where: Optional[Dict[str, Any]]) -> List[qm.FieldCondition]:
    if not where:
        return []
    if "$and" in where and isinstance(where["$and"], list):
        conditions: List[qm.FieldCondition] = []
        for item in where["$and"]:
            conditions.extend(_to_qdrant_filter(item if isinstance(item, dict) else None))
        return conditions

    conds: List[qm.FieldCondition] = []
    for key, value in where.items():
        if key.startswith("$"):
            continue
        conds.append(
            qm.FieldCondition(
                key=f"metadata.{key}",
                match=qm.MatchValue(value=value),
            )
        )
    return conds
