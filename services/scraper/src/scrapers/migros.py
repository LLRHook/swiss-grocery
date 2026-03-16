import logging

from src.models.product import ScrapedProduct
from src.scrapers.base import BaseScraper

logger = logging.getLogger(__name__)


class MigrosScraper(BaseScraper):
    store_slug = "migros"
    store_name = "Migros"
    base_url = "https://www.migros.ch"

    def get_category_urls(self) -> list[str]:
        # TODO: Discover category URLs from Migros sitemap or navigation
        # Migros has an internal product API at /api/
        return []

    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        # TODO: Implement Migros scraping
        # Migros product pages contain structured data that can be parsed
        # Their API endpoints return JSON with product details
        logger.info("Migros scraper not yet implemented")
        return []
