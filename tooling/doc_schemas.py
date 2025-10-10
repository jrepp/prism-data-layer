#!/usr/bin/env python3
"""
Pydantic schemas for Prism documentation frontmatter validation.

These schemas enforce consistent metadata across ADRs, RFCs, and Memos.
"""

import datetime
from typing import List, Literal, Optional
from pydantic import BaseModel, Field, field_validator
import re


class ADRFrontmatter(BaseModel):
    """Schema for Architecture Decision Record frontmatter"""

    title: str = Field(..., min_length=10, description="ADR title (must start with ADR-XXX)")
    status: Literal["Proposed", "Accepted", "Implemented", "Deprecated", "Superseded"] = Field(
        ..., description="Current status of the decision"
    )
    date: datetime.date = Field(..., description="Date of decision (YYYY-MM-DD)")
    deciders: str = Field(..., description="Who made the decision (person or team)")
    tags: List[str] = Field(default_factory=list, description="Lowercase, hyphenated tags")

    @field_validator('title')
    @classmethod
    def validate_title_format(cls, v: str) -> str:
        """Ensure title starts with ADR-XXX"""
        if not re.match(r'^ADR-\d{3}:', v):
            raise ValueError(
                f"ADR title must start with 'ADR-XXX:' format (e.g., 'ADR-001: Title Here'). Got: {v[:50]}"
            )
        return v

    @field_validator('tags')
    @classmethod
    def validate_tags_format(cls, v: List[str]) -> List[str]:
        """Ensure tags are lowercase and hyphenated"""
        for tag in v:
            if not re.match(r'^[a-z0-9\-]+$', tag):
                raise ValueError(
                    f"Invalid tag '{tag}' - tags must be lowercase with hyphens only (e.g., 'data-access', 'backend')"
                )
        return v

    @field_validator('deciders')
    @classmethod
    def validate_deciders(cls, v: str) -> str:
        """Ensure deciders is not empty"""
        if not v.strip():
            raise ValueError("'deciders' field cannot be empty")
        return v


class RFCFrontmatter(BaseModel):
    """Schema for Request for Comments frontmatter"""

    title: str = Field(..., min_length=10, description="RFC title (must start with RFC-XXX)")
    status: Literal["Draft", "Proposed", "Accepted", "Implemented", "Deprecated", "Superseded"] = Field(
        ..., description="Current status of the RFC"
    )
    author: str = Field(..., description="RFC author(s)")
    created: datetime.date = Field(..., description="Creation date (YYYY-MM-DD)")
    updated: Optional[datetime.date] = Field(None, description="Last update date (YYYY-MM-DD)")
    tags: List[str] = Field(default_factory=list, description="Lowercase, hyphenated tags")

    @field_validator('title')
    @classmethod
    def validate_title_format(cls, v: str) -> str:
        """Ensure title starts with RFC-XXX"""
        if not re.match(r'^RFC-\d{3}:', v):
            raise ValueError(
                f"RFC title must start with 'RFC-XXX:' format (e.g., 'RFC-001: Title Here'). Got: {v[:50]}"
            )
        return v

    @field_validator('tags')
    @classmethod
    def validate_tags_format(cls, v: List[str]) -> List[str]:
        """Ensure tags are lowercase and hyphenated"""
        for tag in v:
            if not re.match(r'^[a-z0-9\-]+$', tag):
                raise ValueError(
                    f"Invalid tag '{tag}' - tags must be lowercase with hyphens only (e.g., 'api-design', 'patterns')"
                )
        return v

    @field_validator('author')
    @classmethod
    def validate_author(cls, v: str) -> str:
        """Ensure author is not empty"""
        if not v.strip():
            raise ValueError("'author' field cannot be empty")
        return v


class MemoFrontmatter(BaseModel):
    """Schema for Technical Memo frontmatter"""

    title: str = Field(..., min_length=10, description="Memo title (must start with MEMO-XXX)")
    author: str = Field(..., description="Memo author(s)")
    created: datetime.date = Field(..., description="Creation date (YYYY-MM-DD)")
    updated: datetime.date = Field(..., description="Last update date (YYYY-MM-DD)")
    tags: List[str] = Field(default_factory=list, description="Lowercase, hyphenated tags")

    @field_validator('title')
    @classmethod
    def validate_title_format(cls, v: str) -> str:
        """Ensure title starts with MEMO-XXX"""
        if not re.match(r'^MEMO-\d{3}:', v):
            raise ValueError(
                f"Memo title must start with 'MEMO-XXX:' format (e.g., 'MEMO-001: Title Here'). Got: {v[:50]}"
            )
        return v

    @field_validator('tags')
    @classmethod
    def validate_tags_format(cls, v: List[str]) -> List[str]:
        """Ensure tags are lowercase and hyphenated"""
        for tag in v:
            if not re.match(r'^[a-z0-9\-]+$', tag):
                raise ValueError(
                    f"Invalid tag '{tag}' - tags must be lowercase with hyphens only (e.g., 'architecture', 'design')"
                )
        return v

    @field_validator('author')
    @classmethod
    def validate_author(cls, v: str) -> str:
        """Ensure author is not empty"""
        if not v.strip():
            raise ValueError("'author' field cannot be empty")
        return v


class GenericDocFrontmatter(BaseModel):
    """Schema for general documentation frontmatter (minimal requirements)"""

    title: str = Field(..., min_length=3, description="Document title")
    description: Optional[str] = Field(None, description="Optional description")
    sidebar_position: Optional[int] = Field(None, description="Position in sidebar")
    tags: List[str] = Field(default_factory=list, description="Lowercase, hyphenated tags")

    @field_validator('tags')
    @classmethod
    def validate_tags_format(cls, v: List[str]) -> List[str]:
        """Ensure tags are lowercase and hyphenated"""
        for tag in v:
            if not re.match(r'^[a-z0-9\-]+$', tag):
                raise ValueError(
                    f"Invalid tag '{tag}' - tags must be lowercase with hyphens only"
                )
        return v


# Valid status values for quick reference
VALID_ADR_STATUSES = ["Proposed", "Accepted", "Implemented", "Deprecated", "Superseded"]
VALID_RFC_STATUSES = ["Draft", "Proposed", "Accepted", "Implemented", "Deprecated", "Superseded"]

# Common tag suggestions (not enforced, just for reference)
COMMON_TAGS = [
    "architecture", "backend", "performance", "go", "rust", "testing",
    "reliability", "dx", "operations", "observability", "plugin",
    "cli", "protobuf", "api-design", "deployment", "security",
    "authentication", "patterns", "schemas", "registry"
]
