import logging

from src.models.product import ScrapedProduct
from src.scrapers.base import BaseScraper

logger = logging.getLogger(__name__)


class CoopScraper(BaseScraper):
    store_slug = "coop"
    store_name = "Coop"
    base_url = "https://www.coop.ch"

    def get_category_urls(self) -> list[str]:
        # TODO: Discover category URLs from Coop online shop
        return []

    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        # TODO: Implement Coop scraping
        logger.info("Coop scraper not yet implemented")
        return []
