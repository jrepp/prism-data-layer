import React from 'react';
import type { ReactElement, ReactNode } from 'react';
import {useThemeConfig} from '@docusaurus/theme-common';
import {
  splitNavbarItems,
  useNavbarMobileSidebar,
} from '@docusaurus/theme-common/internal';
import NavbarItem, {type Props as NavbarItemConfig} from '@theme/NavbarItem';
import NavbarColorModeToggle from '@theme/Navbar/ColorModeToggle';
import SearchBar from '@theme/SearchBar';
import NavbarMobileSidebarToggle from '@theme/Navbar/MobileSidebar/Toggle';
import NavbarLogo from '@theme/Navbar/Logo';
import NavbarSearch from '@theme/Navbar/Search';
import BuildInfo from '@site/src/components/BuildInfo';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';

import styles from './styles.module.css';

type CustomNavbarItemConfig = NavbarItemConfig | { type: 'custom-buildInfo'; position: 'left' | 'right' };

function useNavbarItems() {
  // TODO temporary casting until ThemeConfig type is improved
  return useThemeConfig().navbar.items as CustomNavbarItemConfig[];
}

function NavbarItems({items}: {items: CustomNavbarItemConfig[]}): ReactElement {
  const {siteConfig} = useDocusaurusContext();

  return (
    <>
      {items.map((item, i) => {
        // Handle custom buildInfo type
        if ('type' in item && item.type === 'custom-buildInfo') {
          const customFields = siteConfig.customFields as {
            version?: string;
            buildTime?: string;
            commitHash?: string;
          };
          return (
            <BuildInfo
              key={i}
              version={customFields.version}
              buildTime={customFields.buildTime}
              commitHash={customFields.commitHash}
            />
          );
        }

        return <NavbarItem {...(item as NavbarItemConfig)} key={i} />;
      })}
    </>
  );
}

function NavbarContentLayout({
  left,
  right,
}: {
  left: ReactNode;
  right: ReactNode;
}) {
  return (
    <div className="navbar__inner">
      <div className="navbar__items">{left}</div>
      <div className="navbar__items navbar__items--right">{right}</div>
    </div>
  );
}

export default function NavbarContent(): ReactElement {
  const mobileSidebar = useNavbarMobileSidebar();

  const items = useNavbarItems();
  const [leftItems, rightItems] = splitNavbarItems(items);

  const searchBarItem = items.find((item) => item.type === 'search');

  return (
    <NavbarContentLayout
      left={
        <>
          {!mobileSidebar.disabled && <NavbarMobileSidebarToggle />}
          <NavbarLogo />
          <NavbarItems items={leftItems} />
        </>
      }
      right={
        <>
          <NavbarItems items={rightItems} />
          <NavbarColorModeToggle className={styles.colorModeToggle} />
          {!searchBarItem && (
            <NavbarSearch>
              <SearchBar />
            </NavbarSearch>
          )}
        </>
      }
    />
  );
}
