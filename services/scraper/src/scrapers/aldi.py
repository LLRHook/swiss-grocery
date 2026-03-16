import logging

from src.models.product import ScrapedProduct
from src.scrapers.base import BaseScraper

logger = logging.getLogger(__name__)


class AldiScraper(BaseScraper):
    store_slug = "aldi"
    store_name = "Aldi"
    base_url = "https://www.aldi-now.ch"

    def get_category_urls(self) -> list[str]:
        # TODO: Discover category URLs from Aldi
        return []

    def scrape_category(self, category_url: str) -> list[ScrapedProduct]:
        # TODO: Implement Aldi scraping
        logger.info("Aldi scraper not yet implemented")
        return []
