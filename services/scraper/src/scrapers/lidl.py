import logging

from src.models.product import ScrapedProduct
from src.scrapers.base import BaseScraper

logger = logging.getLogger(__name__)


class LidlScraper(BaseScraper):
    store_slug = "lidl"
    store_name = "Lidl"
    base_url = "https://www.lidl.ch"

    def get_category_urls(self) -> list[str]:
        # TODO: Discover category URLs from Lidl
        return []

    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        # TODO: Implement Lidl scraping
        logger.info("Lidl scraper not yet implemented")
        return []
